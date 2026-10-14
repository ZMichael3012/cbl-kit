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

// Publisher публикует конверт во внешнюю шину.
type Publisher interface {
	// Publish отправляет событие в топик; ошибка означает, что событие
	// осталось неопубликованным и будет повторено следующим циклом relay.
	Publish(ctx context.Context, env Envelope) error
}

// Metrics описывает метрики relay; реализация по умолчанию ничего не делает.
type Metrics interface {
	// AddPublished учитывает опубликованные события по топику.
	AddPublished(n int, topic string)
	// SetPending устанавливает число неопубликованных событий по топику.
	SetPending(n int, topic string)
}

type noopMetrics struct{}

func (noopMetrics) AddPublished(int, string) {}
func (noopMetrics) SetPending(int, string)   {}

// Relay публикует неопубликованные события из outbox.
type Relay struct {
	db       *gorm.DB
	pub      Publisher
	log      *zap.Logger
	batch    int
	interval time.Duration
	metrics  Metrics
}

// Option настраивает Relay.
type Option func(*Relay)

// WithBatch задает размер пачки публикации.
func WithBatch(n int) Option {
	return func(r *Relay) {
		if n > 0 {
			r.batch = n
		}
	}
}

// WithInterval задает интервал цикла публикации.
func WithInterval(d time.Duration) Option {
	return func(r *Relay) {
		if d > 0 {
			r.interval = d
		}
	}
}

// WithMetrics подключает метрики relay.
func WithMetrics(m Metrics) Option {
	return func(r *Relay) {
		if m != nil {
			r.metrics = m
		}
	}
}

// NewRelay создает relay публикации outbox.
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

// PublishBatch публикует неопубликованные события пачками до опустошения outbox.
// Строки захватываются через FOR UPDATE SKIP LOCKED, поэтому несколько relay
// могут работать конкурентно. При ошибке публикации транзакция откатывается,
// и события остаются неопубликованными для следующего цикла.
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

// Run публикует события до отмены контекста.
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

// updatePending публикует метрику числа неопубликованных событий по топикам.
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
