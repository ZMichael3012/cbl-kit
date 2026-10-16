package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Local is an in-process token bucket. It cannot coordinate replicas, so it
// serves as a fallback when Redis is unavailable and must be configured
// conservatively (each replica holds its own bucket). The bucket map keeps one
// entry per key and is intended for a bounded key set (providers, credentials).
type Local struct {
	cfg     Config
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]*bucketState
}

// bucketState carries the fixed-point tokens and the baseline time; a baseline
// in the future means the bucket was paused.
type bucketState struct {
	tokens  int64
	updated time.Time
}

// NewLocal creates a local limiter with the system clock.
func NewLocal(cfg Config) (*Local, error) {
	return NewLocalWithClock(cfg, time.Now)
}

// NewLocalWithClock creates a local limiter with an injected clock for tests.
func NewLocalWithClock(cfg Config, now func() time.Time) (*Local, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, fmt.Errorf("%w: nil clock", ErrInvalidConfig)
	}
	return &Local{cfg: cfg, now: now, buckets: make(map[string]*bucketState)}, nil
}

// Allow implements Limiter.
func (l *Local) Allow(_ context.Context, key string) (Decision, error) {
	if err := validateKey(key); err != nil {
		return Decision{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	state := l.bucket(key, now)
	l.refill(state, now)
	if state.tokens >= tokenScale {
		state.tokens -= tokenScale
		return Decision{Allowed: true, Remaining: int(state.tokens / tokenScale)}, nil
	}
	return Decision{Allowed: false, RetryAfter: l.wait(state, now)}, nil
}

// Pause implements Limiter: the bucket is emptied and its baseline moves so the
// first token after the pause appears exactly when d elapses.
func (l *Local) Pause(_ context.Context, key string, d time.Duration) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if d < 0 {
		return fmt.Errorf("%w: %s", ErrInvalidPause, d)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	state := l.bucket(key, now)
	state.tokens = 0
	state.updated = now.Add(d - l.cfg.RefillInterval)
	return nil
}

// bucket returns the state of a key, creating a full bucket on first use.
func (l *Local) bucket(key string, now time.Time) *bucketState {
	state, ok := l.buckets[key]
	if !ok {
		state = &bucketState{tokens: int64(l.cfg.Burst) * tokenScale, updated: now}
		l.buckets[key] = state
	}
	return state
}

// refill adds tokens for the elapsed time; while the bucket is paused the
// baseline lies in the future and no tokens are added. Whole refill steps are
// counted first and partial time is converted through 128-bit math, so an
// arbitrarily long idle period cannot overflow int64.
func (l *Local) refill(state *bucketState, now time.Time) {
	elapsed := now.Sub(state.updated)
	if elapsed <= 0 {
		return
	}
	interval := int64(l.cfg.RefillInterval)
	steps := int64(elapsed) / interval
	if steps >= int64(l.cfg.Burst) {
		state.tokens = int64(l.cfg.Burst) * tokenScale
		state.updated = now
		return
	}
	added := steps * tokenScale
	consumed := steps * interval
	partial := int64(elapsed) - consumed
	if partial > 0 {
		extra := mulDiv(partial, tokenScale, interval)
		added += extra
		consumed += mulDiv(extra, interval, tokenScale)
	}
	state.tokens += added
	// Advance the baseline only by the time converted into whole tokens to keep
	// the fractional part for the next refill.
	state.updated = state.updated.Add(time.Duration(consumed))
}

// wait computes the deny delay of the current state.
func (l *Local) wait(state *bucketState, now time.Time) time.Duration {
	var paused time.Duration
	if state.updated.After(now) {
		paused = state.updated.Sub(now)
	}
	return retryAfter(state.tokens, l.cfg.RefillInterval, paused)
}
