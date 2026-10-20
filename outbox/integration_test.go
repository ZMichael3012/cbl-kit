//go:build integration

package outbox_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ZMichael3012/cbl-kit/outbox"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []outbox.Envelope
	failNext  bool
}

func (p *recordingPublisher) Publish(_ context.Context, env outbox.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		return fmt.Errorf("simulated kafka failure")
	}
	p.published = append(p.published, env)
	return nil
}

func (p *recordingPublisher) events() []outbox.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]outbox.Envelope(nil), p.published...)
}

func setupOutboxDB(t *testing.T) *gorm.DB {
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

	ddl, err := os.ReadFile(filepath.Join("..", "migrations", "outbox.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(ddl)).Error)
	return db
}

func appendEnvelope(t *testing.T, db *gorm.DB, topic string, n int) outbox.Envelope {
	t.Helper()
	env, err := outbox.NewEnvelope("payment.completed", topic, fmt.Sprintf("key-%d", n), nil, []byte(fmt.Sprintf(`{"n":%d}`, n)))
	require.NoError(t, err)
	require.NoError(t, outbox.NewStore().Append(context.Background(), db, env))
	return env
}

func TestIntegrationAppendAndRelay(t *testing.T) {
	db := setupOutboxDB(t)
	pub := &recordingPublisher{}
	relay := outbox.NewRelay(db, pub, zap.NewNop(), outbox.WithBatch(10))

	env := appendEnvelope(t, db, "payment.events", 1)

	n, err := relay.PublishBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	events := pub.events()
	require.Len(t, events, 1)
	require.Equal(t, env.EventID, events[0].EventID)
	require.Equal(t, "payment.events", events[0].Topic)

	n, err = relay.PublishBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestIntegrationRollbackKeepsNothing(t *testing.T) {
	db := setupOutboxDB(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		env, envErr := outbox.NewEnvelope("payment.completed", "payment.events", "k", nil, []byte("{}"))
		if envErr != nil {
			return envErr
		}
		if appendErr := outbox.NewStore().Append(context.Background(), tx, env); appendErr != nil {
			return appendErr
		}
		return fmt.Errorf("business failure")
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&outbox.Record{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestIntegrationPublishFailureKeepsEvent(t *testing.T) {
	db := setupOutboxDB(t)
	pub := &recordingPublisher{failNext: true}
	relay := outbox.NewRelay(db, pub, zap.NewNop())

	appendEnvelope(t, db, "ledger.events", 1)

	_, err := relay.PublishBatch(context.Background())
	require.Error(t, err)

	var pending int64
	require.NoError(t, db.Model(&outbox.Record{}).Where("published_at IS NULL").Count(&pending).Error)
	require.Equal(t, int64(1), pending)

	n, err := relay.PublishBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var published int64
	require.NoError(t, db.Model(&outbox.Record{}).Where("published_at IS NOT NULL").Count(&published).Error)
	require.Equal(t, int64(1), published)
}

func TestIntegrationConcurrentRelaysPublishExactlyOnce(t *testing.T) {
	db := setupOutboxDB(t)
	pub := &recordingPublisher{}
	relayA := outbox.NewRelay(db, pub, zap.NewNop(), outbox.WithBatch(5))
	relayB := outbox.NewRelay(db, pub, zap.NewNop(), outbox.WithBatch(5))

	const total = 40
	for i := 0; i < total; i++ {
		appendEnvelope(t, db, "payment.events", i)
	}

	var wg sync.WaitGroup
	for _, relay := range []*outbox.Relay{relayA, relayB} {
		wg.Add(1)
		go func(r *outbox.Relay) {
			defer wg.Done()
			_, _ = r.PublishBatch(context.Background())
		}(relay)
	}
	wg.Wait()

	seen := map[string]int{}
	for _, env := range pub.events() {
		seen[env.EventID]++
	}
	require.Len(t, seen, total)
	for id, n := range seen {
		require.Equalf(t, 1, n, "event %s published %d times", id, n)
	}
}
