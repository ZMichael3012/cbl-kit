package ratelimit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/ratelimit"
)

type fakeLimiter struct {
	decision  ratelimit.Decision
	err       error
	pauseErr  error
	allowHits int
	pauseHits int
}

func (f *fakeLimiter) Allow(_ context.Context, _ string) (ratelimit.Decision, error) {
	f.allowHits++
	return f.decision, f.err
}

func (f *fakeLimiter) Pause(_ context.Context, _ string, _ time.Duration) error {
	f.pauseHits++
	return f.pauseErr
}

type fakeMetrics struct {
	allowed  int
	denied   int
	errors   int
	fallback int
}

func (m *fakeMetrics) ObserveAllowed(_ string, _ int) { m.allowed++ }

func (m *fakeMetrics) ObserveDenied(_ string, _ time.Duration) { m.denied++ }

func (m *fakeMetrics) ObserveError(_ string, _ error, fallback bool) {
	m.errors++
	if fallback {
		m.fallback++
	}
}

func TestWithFallbackUsesPrimary(t *testing.T) {
	t.Parallel()
	primary := &fakeLimiter{decision: ratelimit.Decision{Allowed: true, Remaining: 3}}
	fallback := &fakeLimiter{decision: ratelimit.Decision{Allowed: true}}
	metrics := &fakeMetrics{}

	limiter, err := ratelimit.NewWithFallback(primary, fallback, metrics)
	require.NoError(t, err)

	got, err := limiter.Allow(context.Background(), "coingate:invoice")
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, 1, primary.allowHits)
	require.Zero(t, fallback.allowHits)
	require.Equal(t, 1, metrics.allowed)

	primary.decision = ratelimit.Decision{Allowed: false, RetryAfter: time.Second}
	_, err = limiter.Allow(context.Background(), "coingate:invoice")
	require.NoError(t, err)
	require.Equal(t, 1, metrics.denied)
}

func TestWithFallbackSwitchesOnError(t *testing.T) {
	t.Parallel()
	primary := &fakeLimiter{err: errors.New("redis down")}
	fallback := &fakeLimiter{decision: ratelimit.Decision{Allowed: true}}
	metrics := &fakeMetrics{}

	limiter, err := ratelimit.NewWithFallback(primary, fallback, metrics)
	require.NoError(t, err)

	got, err := limiter.Allow(context.Background(), "coingate:invoice")
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, 1, primary.allowHits)
	require.Equal(t, 1, fallback.allowHits)
	require.Equal(t, 1, metrics.errors)
	require.Equal(t, 1, metrics.fallback)
	require.Equal(t, 1, metrics.allowed)
}

func TestWithFallbackPause(t *testing.T) {
	t.Parallel()
	primary := &fakeLimiter{}
	fallback := &fakeLimiter{}
	limiter, err := ratelimit.NewWithFallback(primary, fallback, nil)
	require.NoError(t, err)

	require.NoError(t, limiter.Pause(context.Background(), "key", time.Second))
	require.Equal(t, 1, primary.pauseHits)
	require.Zero(t, fallback.pauseHits)

	primary.pauseErr = errors.New("redis down")
	require.NoError(t, limiter.Pause(context.Background(), "key", time.Second))
	require.Equal(t, 1, fallback.pauseHits)
}

func TestWithFallbackValidation(t *testing.T) {
	t.Parallel()
	_, err := ratelimit.NewWithFallback(nil, &fakeLimiter{}, nil)
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
	_, err = ratelimit.NewWithFallback(&fakeLimiter{}, nil, nil)
	require.ErrorIs(t, err, ratelimit.ErrInvalidConfig)
}

func TestWithFallbackValidatesKey(t *testing.T) {
	t.Parallel()
	limiter, err := ratelimit.NewWithFallback(&fakeLimiter{}, &fakeLimiter{}, nil)
	require.NoError(t, err)
	_, err = limiter.Allow(context.Background(), "")
	require.ErrorIs(t, err, ratelimit.ErrInvalidKey)
}
