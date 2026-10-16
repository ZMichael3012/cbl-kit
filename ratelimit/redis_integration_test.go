//go:build integration

package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/cbl/cbl-kit/ratelimit"
)

func TestRedisLimiterIntegration(t *testing.T) {
	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, testcontainers.TerminateContainer(container))
	}()

	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: endpoint})
	defer func() { _ = client.Close() }()

	limiter, err := ratelimit.NewRedis(client, ratelimit.Config{RefillInterval: time.Second, Burst: 2})
	require.NoError(t, err)

	first, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, first.Allowed)
	require.Equal(t, 1, first.Remaining)

	second, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, second.Allowed)

	third, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, third.Allowed)
	require.Greater(t, third.RetryAfter, time.Duration(0))

	require.NoError(t, limiter.Pause(ctx, "coingate:invoice", 300*time.Millisecond))
	paused, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, paused.Allowed)
	require.GreaterOrEqual(t, paused.RetryAfter, 250*time.Millisecond)

	time.Sleep(1100 * time.Millisecond)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}
