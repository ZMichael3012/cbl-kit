package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/ratelimit"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newLocalLimiter(t *testing.T, burst int, interval time.Duration) (*ratelimit.Local, *fakeClock) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	limiter, err := ratelimit.NewLocalWithClock(ratelimit.Config{RefillInterval: interval, Burst: burst}, clock.Now)
	require.NoError(t, err)
	return limiter, clock
}

func TestLocalConfigValidation(t *testing.T) {
	t.Parallel()
	_, err := ratelimit.NewLocal(ratelimit.Config{Burst: 1})
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
	_, err = ratelimit.NewLocal(ratelimit.Config{RefillInterval: time.Second})
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
	_, err = ratelimit.NewLocalWithClock(ratelimit.Config{RefillInterval: time.Second, Burst: 1}, nil)
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
}

func TestLocalBurstAndRefill(t *testing.T) {
	t.Parallel()
	limiter, clock := newLocalLimiter(t, 2, time.Second)
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

	clock.Advance(500 * time.Millisecond)
	half, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, half.Allowed)
	require.Equal(t, 500*time.Millisecond, half.RetryAfter)

	clock.Advance(500 * time.Millisecond)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}

func TestLocalKeysAreIsolated(t *testing.T) {
	t.Parallel()
	limiter, _ := newLocalLimiter(t, 1, time.Second)
	ctx := context.Background()

	first, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, first.Allowed)

	other, err := limiter.Allow(ctx, "btcpay:invoice")
	require.NoError(t, err)
	require.True(t, other.Allowed)

	again, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, again.Allowed)
}

func TestLocalBurstCapAfterIdle(t *testing.T) {
	t.Parallel()
	limiter, clock := newLocalLimiter(t, 3, time.Second)
	ctx := context.Background()

	clock.Advance(4 * time.Hour)
	got, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, 2, got.Remaining)
}

func TestLocalLongInterval(t *testing.T) {
	t.Parallel()
	limiter, clock := newLocalLimiter(t, 1, 4*time.Hour)
	ctx := context.Background()

	first, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.True(t, first.Allowed)

	denied, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.False(t, denied.Allowed)
	require.Equal(t, 4*time.Hour, denied.RetryAfter)

	clock.Advance(8 * time.Hour)
	recovered, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}

func TestLocalZeroPause(t *testing.T) {
	t.Parallel()
	limiter, _ := newLocalLimiter(t, 5, time.Second)
	ctx := context.Background()

	require.NoError(t, limiter.Pause(ctx, "key", 0))
	got, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, 0, got.Remaining)
}

func TestLocalPause(t *testing.T) {
	t.Parallel()
	limiter, clock := newLocalLimiter(t, 5, time.Second)
	ctx := context.Background()

	got, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, got.Allowed)

	require.NoError(t, limiter.Pause(ctx, "coingate:invoice", 2*time.Second))

	denied, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, denied.Allowed)
	require.Equal(t, 2*time.Second, denied.RetryAfter)

	clock.Advance(1900 * time.Millisecond)
	still, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.False(t, still.Allowed)

	clock.Advance(100 * time.Millisecond)
	recovered, err := limiter.Allow(ctx, "coingate:invoice")
	require.NoError(t, err)
	require.True(t, recovered.Allowed)
}

func TestLocalPauseShorterThanInterval(t *testing.T) {
	t.Parallel()
	limiter, clock := newLocalLimiter(t, 5, time.Second)
	ctx := context.Background()

	require.NoError(t, limiter.Pause(ctx, "key", 300*time.Millisecond))
	denied, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.False(t, denied.Allowed)
	require.Equal(t, 300*time.Millisecond, denied.RetryAfter)

	clock.Advance(300 * time.Millisecond)
	got, err := limiter.Allow(ctx, "key")
	require.NoError(t, err)
	require.True(t, got.Allowed)
}

func TestLocalPauseValidation(t *testing.T) {
	t.Parallel()
	limiter, _ := newLocalLimiter(t, 1, time.Second)
	ctx := context.Background()
	require.ErrorIs(t, limiter.Pause(ctx, "key", -time.Second), ratelimit.ErrInvalidPause)
	_, err := limiter.Allow(ctx, "")
	require.ErrorIs(t, err, ratelimit.ErrInvalidKey)
	require.ErrorIs(t, limiter.Pause(ctx, "", time.Second), ratelimit.ErrInvalidKey)
}
