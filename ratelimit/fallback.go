package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// nopMetrics discards limiter events when no metrics implementation is injected.
type nopMetrics struct{}

// ObserveAllowed implements Metrics.
func (nopMetrics) ObserveAllowed(_ string, _ int) {}

// ObserveDenied implements Metrics.
func (nopMetrics) ObserveDenied(_ string, _ time.Duration) {}

// ObserveError implements Metrics.
func (nopMetrics) ObserveError(_ string, _ error, _ bool) {}

// WithFallback routes requests to the primary limiter and switches to the
// fallback when the primary fails; the switch is reported through Metrics.
// The fallback is per-process, so it must be configured conservatively.
type WithFallback struct {
	primary  Limiter
	fallback Limiter
	metrics  Metrics
}

// NewWithFallback creates a fallback limiter; a nil limiter is an error, a nil
// metrics value is replaced by a no-op implementation.
func NewWithFallback(primary, fallback Limiter, metrics Metrics) (*WithFallback, error) {
	if primary == nil || fallback == nil {
		return nil, fmt.Errorf("%w: nil limiter", ErrInvalidConfig)
	}
	if metrics == nil {
		metrics = nopMetrics{}
	}
	return &WithFallback{primary: primary, fallback: fallback, metrics: metrics}, nil
}

// Allow implements Limiter.
func (w *WithFallback) Allow(ctx context.Context, key string) (Decision, error) {
	if err := validateKey(key); err != nil {
		return Decision{}, err
	}
	decision, err := w.primary.Allow(ctx, key)
	if err != nil {
		w.metrics.ObserveError(key, err, true)
		decision, err = w.fallback.Allow(ctx, key)
		if err != nil {
			return Decision{}, err
		}
		w.report(key, decision)
		return decision, nil
	}
	w.report(key, decision)
	return decision, nil
}

// report records the served decision, whichever limiter produced it.
func (w *WithFallback) report(key string, decision Decision) {
	if decision.Allowed {
		w.metrics.ObserveAllowed(key, decision.Remaining)
	} else {
		w.metrics.ObserveDenied(key, decision.RetryAfter)
	}
}

// Pause implements Limiter.
func (w *WithFallback) Pause(ctx context.Context, key string, d time.Duration) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := w.primary.Pause(ctx, key, d); err != nil {
		w.metrics.ObserveError(key, err, true)
		return w.fallback.Pause(ctx, key, d)
	}
	return nil
}
