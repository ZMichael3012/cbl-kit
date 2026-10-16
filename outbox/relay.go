package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Publisher publishes an envelope to the external bus.
type Publisher interface {
	// Publish sends the event to the topic; an error means that the event
	// remained unpublished and will be retried by the next relay cycle.
	Publish(ctx context.Context, env Envelope) error
}

// Metrics describes the relay metrics; the default implementation does nothing.
type Metrics interface {
	// AddPublished accounts for published events by topic.
	AddPublished(n int, topic string)
	// SetPending sets the number of unpublished events by topic.
	SetPending(n int, topic string)
}

type noopMetrics struct{}

func (noopMetrics) AddPublished(int, string) {}
func (noopMetrics) SetPending(int, string)   {}

// Relay publishes unpublished events from the outbox.
type Relay struct {
	db       *gorm.DB
	pub      Publisher
	log      *zap.Logger
	batch    int
	interval time.Duration
	metrics  Metrics
}

// Option configures Relay.
type Option func(*Relay)

// WithBatch sets the publication batch size.
func WithBatch(n int) Option {
	return func(r *Relay) {
		if n > 0 {
			r.batch = n
		}
	}
}

// WithInterval sets the publication cycle interval.
func WithInterval(d time.Duration) Option {
	return func(r *Relay) {
		if d > 0 {
			r.interval = d
		}
	}
}

// WithMetrics attaches the relay metrics.
func WithMetrics(m Metrics) Option {
	return func(r *Relay) {
		if m != nil {
			r.metrics = m
		}
	}
}

// NewRelay creates an outbox publication relay.
func NewRelay(db *gorm.DB, pub Publisher, log *zap.Logger, opts ...Option) *Relay {
	if log == nil {
		log = zap.NewNop()
	}
	r := &Relay{
		db:       db,
		pub:      pub,
		log:      log,
		batch:    100,
		interval: 250 * time.Millisecond,
		metrics:  noopMetrics{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// PublishBatch publishes unpublished events in batches until the outbox is empty.
// Rows are locked with FOR UPDATE SKIP LOCKED, so several relays can run
// concurrently. On a publication error the transaction is rolled back and the
// events remain unpublished for the next cycle.
func (r *Relay) PublishBatch(ctx context.Context) (int, error) {
	if r.db == nil || r.pub == nil {
		return 0, errors.New("outbox: relay is not configured")
	}
	total := 0
	for {
		var records []Record
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("published_at IS NULL").
				Order("id").
				Limit(r.batch).
				Find(&records).Error; err != nil {
				return err
			}
			if len(records) == 0 {
				return nil
			}
			now := time.Now().UTC()
			for i := range records {
				env := Envelope{
					EventID: records[i].EventID,
					Topic:   records[i].Topic,
					Key:     records[i].Key,
					Headers: map[string]string(records[i].Headers),
					Payload: records[i].Payload,
				}
				if err := r.pub.Publish(ctx, env); err != nil {
					return fmt.Errorf("outbox: publish %s: %w", env.EventID, err)
				}
				if err := tx.Model(&Record{}).
					Where("id = ?", records[i].ID).
					Update("published_at", now).Error; err != nil {
					return fmt.Errorf("outbox: mark published %d: %w", records[i].ID, err)
				}
				r.metrics.AddPublished(1, env.Topic)
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		n := len(records)
		total += n
		if n < r.batch {
			r.updatePending(ctx)
			return total, nil
		}
	}
}

// Run publishes events until the context is canceled.
func (r *Relay) Run(ctx context.Context) error {
	if r.db == nil || r.pub == nil {
		return errors.New("outbox: relay is not configured")
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		n, err := r.PublishBatch(ctx)
		switch {
		case err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
			return err
		case err != nil:
			r.log.Error("outbox publish batch failed", zap.Error(err))
		case n > 0:
			r.log.Info("outbox published batch", zap.Int("count", n))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// updatePending emits the metric of unpublished event counts per topic.
func (r *Relay) updatePending(ctx context.Context) {
	type row struct {
		Topic string
		N     int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&Record{}).
		Select("topic, count(*) as n").
		Where("published_at IS NULL").
		Group("topic").
		Scan(&rows).Error
	if err != nil {
		r.log.Debug("outbox pending metric failed", zap.Error(err))
		return
	}
	for _, row := range rows {
		r.metrics.SetPending(int(row.N), row.Topic)
	}
}
