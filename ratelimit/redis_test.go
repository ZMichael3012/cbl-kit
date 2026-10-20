package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/ratelimit"
)

func newRedisLimiter(t *testing.T, burst int, interval time.Duration) (*ratelimit.Redis, *miniredis.Miniredis, *fakeClock) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	limiter, err := ratelimit.NewRedisWithClock(client, ratelimit.Config{RefillInterval: interval, Burst: burst}, clock.Now)
	require.NoError(t, err)
	return limiter, server, clock
}

func TestRedisBurstAndRefill(t *testing.T) {
	t.Parallel()
	limiter, server, clock := newRedisLimiter(t, 2, time.Second)
	ctx := context.Background()

	first, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, first.Allowed)
	require.Equal(t, 1, first.Remaining)

	second, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, second.Allowed)
	require.Equal(t, 0, second.Remaining)

	third, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, third.Allowed)
	require.Equal(t, time.Second, third.RetryAfter)

	require.Contains(t, server.Keys(), "rl:provider:coingate:invoice")
	require.Greater(t, server.TTL("rl:provider:coingate:invoice"), time.Duration(0))

	clock.Advance(time.Second)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}

func TestRedisStateIsSharedBetweenInstances(t *testing.T) {
	t.Parallel()
	limiter, server, clock := newRedisLimiter(t, 2, time.Second)
	otherClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = otherClient.Close() })
	other, err := ratelimit.NewRedisWithClock(otherClient, ratelimit.Config{RefillInterval: time.Second, Burst: 2}, clock.Now)
	require.NoError(t, err)
	ctx := context.Background()

	for range 2 {
		got, err := limiter.Allow(ctx, "shared")
		require.NoError(t, err)
		require.True(t, got.Allowed)
	}
	got, err := other.Allow(ctx, "shared")
	require.NoError(t, err)
	require.False(t, got.Allowed)
}

func TestRedisPause(t *testing.T) {
	t.Parallel()
	limiter, server, clock := newRedisLimiter(t, 5, time.Second)
	ctx := context.Background()

	require.NoError(t, limiter.Pause(ctx, "coingate:invoice", 2*time.Second))
	denied, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, denied.Allowed)
	require.Equal(t, 2*time.Second, denied.RetryAfter)

	clock.Advance(2 * time.Second)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)

	server.FastForward(time.Second)
}

func TestRedisPauseSurvivesAllowTTLRefresh(t *testing.T) {
	t.Parallel()
	limiter, server, clock := newRedisLimiter(t, 2, time.Second)
	ctx := context.Background()

	require.NoError(t, limiter.Pause(ctx, "coingate:invoice", 30*time.Second))
	denied, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, denied.Allowed)

	// The allow call must not shorten the key retention below the pause: a
	// shorter TTL would let the bucket expire and reset, bypassing the pause.
	require.Greater(t, server.TTL("rl:provider:coingate:invoice"), 29*time.Second)

	clock.Advance(30 * time.Second)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}

func TestRedisValidation(t *testing.T) {
	t.Parallel()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	_, err := ratelimit.NewRedis(client, ratelimit.Config{Burst: 1})
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
	_, err = ratelimit.NewRedis(nil, ratelimit.Config{RefillInterval: time.Second, Burst: 1})
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)

	limiter, err := ratelimit.NewRedis(client, ratelimit.Config{RefillInterval: time.Second, Burst: 1})
	require.NoError(t, err)
	_, err = limiter.Allow(context.Background(), "")
	require.ErrorIs(t, err, ratelimit.ErrInvalidKey)
	require.ErrorIs(t, limiter.Pause(context.Background(), "key", -time.Second), ratelimit.ErrInvalidPause)
}
