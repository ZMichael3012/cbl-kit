//go:build integration

package inbox_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/cbl/cbl-kit/inbox"
	"github.com/cbl/cbl-kit/outbox"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []outbox.Envelope
}

func (p *recordingPublisher) Publish(_ context.Context, env outbox.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, env)
	return nil
}

func (p *recordingPublisher) events() []outbox.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]outbox.Envelope(nil), p.published...)
}

type flakyHandler struct {
	calls     int
	failTimes int
}

func (h *flakyHandler) Handle(_ context.Context, _ *gorm.DB, _ inbox.Message) error {
	h.calls++
	if h.calls <= h.failTimes {
		return fmt.Errorf("transient failure %d", h.calls)
	}
	return nil
}

type effectHandler struct{}

func (effectHandler) Handle(_ context.Context, tx *gorm.DB, _ inbox.Message) error {
	if err := tx.Exec("INSERT INTO test_effect (note) VALUES ('x')").Error; err != nil {
		return err
	}
	return fmt.Errorf("effect then failure")
}

func setupInboxDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("kit"),
		tcpostgres.WithUsername("cbl"),
		tcpostgres.WithPassword("cbl"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	ddl, err := os.ReadFile(filepath.Join("..", "migrations", "inbox.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(ddl)).Error)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS test_effect (note text)").Error)
	return db
}

func newTestMessage(t *testing.T, topic string) inbox.Message {
	t.Helper()
	env, err := outbox.NewEnvelope("payment.completed", topic, "pay-1", nil, []byte(`{"id":"pay-1"}`))
	require.NoError(t, err)
	return inbox.Message{Topic: env.Topic, Key: env.Key, Headers: env.Headers, Value: env.Payload}
}

func TestIntegrationRetryProgressionToDLQ(t *testing.T) {
	db := setupInboxDB(t)
	handler := &flakyHandler{failTimes: 10}
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), handler, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	msg := newTestMessage(t, "payment.events")
	require.NoError(t, processor.Process(context.Background(), msg))

	events := pub.events()
	require.Len(t, events, 1)
	assert.Equal(t, "payment.events.retry.5s", events[0].Topic)
	assert.Equal(t, "1", events[0].Headers[inbox.HeaderRetryAttempt])

	msg = inbox.Message{Topic: events[0].Topic, Key: events[0].Key, Headers: events[0].Headers, Value: events[0].Payload}
	require.NoError(t, processor.Process(context.Background(), msg))
	events = pub.events()
	require.Len(t, events, 2)
	assert.Equal(t, "payment.events.retry.1m", events[1].Topic)
	assert.Equal(t, "2", events[1].Headers[inbox.HeaderRetryAttempt])

	msg = inbox.Message{Topic: events[1].Topic, Key: events[1].Key, Headers: events[1].Headers, Value: events[1].Payload}
	require.NoError(t, processor.Process(context.Background(), msg))
	events = pub.events()
	require.Len(t, events, 3)
	assert.Equal(t, "payment.events.retry.10m", events[2].Topic)

	msg = inbox.Message{Topic: events[2].Topic, Key: events[2].Key, Headers: events[2].Headers, Value: events[2].Payload}
	require.NoError(t, processor.Process(context.Background(), msg))
	events = pub.events()
	require.Len(t, events, 4)
	assert.Equal(t, "payment.events.dlq", events[3].Topic)
	assert.Equal(t, "4", events[3].Headers[inbox.HeaderRetryAttempt])
	assert.NotEmpty(t, events[3].Headers[inbox.HeaderError])

	var inboxCount int64
	require.NoError(t, db.Model(&inbox.Record{}).Count(&inboxCount).Error)
	assert.Equal(t, int64(0), inboxCount)
	assert.Equal(t, 4, handler.calls)
}

func TestIntegrationDuplicateSkipped(t *testing.T) {
	db := setupInboxDB(t)
	handler := &flakyHandler{}
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), handler, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	msg := newTestMessage(t, "payment.events")
	require.NoError(t, processor.Process(context.Background(), msg))
	require.NoError(t, processor.Process(context.Background(), msg))

	assert.Equal(t, 1, handler.calls)
	assert.Empty(t, pub.events())

	var inboxCount int64
	require.NoError(t, db.Model(&inbox.Record{}).Count(&inboxCount).Error)
	assert.Equal(t, int64(1), inboxCount)
}

func TestIntegrationHandlerRollbackIsAtomic(t *testing.T) {
	db := setupInboxDB(t)
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), effectHandler{}, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	msg := newTestMessage(t, "payment.events")
	require.NoError(t, processor.Process(context.Background(), msg))

	var effectCount int64
	require.NoError(t, db.Raw("SELECT count(*) FROM test_effect").Scan(&effectCount).Error)
	assert.Equal(t, int64(0), effectCount)

	var inboxCount int64
	require.NoError(t, db.Model(&inbox.Record{}).Count(&inboxCount).Error)
	assert.Equal(t, int64(0), inboxCount)

	events := pub.events()
	require.Len(t, events, 1)
	assert.Equal(t, "payment.events.retry.5s", events[0].Topic)
}

func TestIntegrationPoisonMessageToDLQ(t *testing.T) {
	db := setupInboxDB(t)
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), &flakyHandler{}, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	msg := inbox.Message{
		Topic:   "payment.events",
		Key:     "pay-1",
		Headers: map[string]string{"event_type": "payment.completed"},
		Value:   []byte("{}"),
	}
	require.NoError(t, processor.Process(context.Background(), msg))

	events := pub.events()
	require.Len(t, events, 1)
	assert.Equal(t, "payment.events.dlq", events[0].Topic)
	assert.NotEmpty(t, events[0].Headers[inbox.HeaderError])

	var inboxCount int64
	require.NoError(t, db.Model(&inbox.Record{}).Count(&inboxCount).Error)
	assert.Equal(t, int64(0), inboxCount)
}

func TestIntegrationInvalidEventIDToDLQ(t *testing.T) {
	db := setupInboxDB(t)
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), &flakyHandler{}, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	// event_id is not a UUID: the retry envelope cannot be built, so the message
	// must go to the DLQ with a sanitized event_id instead of looping the partition.
	msg := inbox.Message{
		Topic: "payment.events",
		Key:   "pay-1",
		Headers: map[string]string{
			"event_id":   "pay-1",
			"event_type": "payment.completed",
		},
		Value: []byte("{}"),
	}
	require.NoError(t, processor.Process(context.Background(), msg))

	events := pub.events()
	require.Len(t, events, 1)
	assert.Equal(t, "payment.events.dlq", events[0].Topic)
	assert.Equal(t, "pay-1", events[0].Headers[inbox.HeaderOriginalEventID])
	assert.NotEqual(t, "pay-1", events[0].EventID, "the DLQ envelope receives a valid event_id")
	assert.NotEmpty(t, events[0].Headers[inbox.HeaderError])
}

func TestIntegrationMissingEventTypeToDLQ(t *testing.T) {
	db := setupInboxDB(t)
	pub := &recordingPublisher{}
	processor := inbox.NewProcessor("payment", db, inbox.NewGuard(), &flakyHandler{failTimes: 10}, pub, inbox.NewRouter("payment.events", 3), zap.NewNop())

	_, err := outbox.NewEnvelope("", "payment.events", "pay-1", nil, []byte("{}"))
	require.Error(t, err, "an envelope without event_type cannot be built")

	msg := inbox.Message{
		Topic: "payment.events",
		Key:   "pay-1",
		Headers: map[string]string{
			"event_id": uuid.Must(uuid.NewV7()).String(),
		},
		Value: []byte("{}"),
	}
	require.NoError(t, processor.Process(context.Background(), msg))

	events := pub.events()
	require.Len(t, events, 1)
	assert.Equal(t, "payment.events.dlq", events[0].Topic)
	assert.Equal(t, "unknown", events[0].Headers[outbox.HeaderEventType])
	assert.NotEmpty(t, events[0].Headers[inbox.HeaderError])
}
