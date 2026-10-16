// Package ratelimit implements a token bucket rate limiter shared through Redis
// with an in-process fallback. The limiter carries no domain logic and uses no
// floats: tokens are counted in fixed-point micro-units, and elapsed time is
// converted exactly with integer arithmetic.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Package errors are returned wrapped through fmt.Errorf with %w.
var (
	// ErrInvalidConfig reports a non-positive refill interval or burst, or a nil dependency.
	ErrInvalidConfig = errors.New("ratelimit: invalid config")
	// ErrInvalidKey reports an empty limiter key.
	ErrInvalidKey = errors.New("ratelimit: empty key")
	// ErrInvalidPause reports a negative pause duration.
	ErrInvalidPause = errors.New("ratelimit: negative pause")
	// ErrInvalidRetryAfter reports an unparsable Retry-After value.
	ErrInvalidRetryAfter = errors.New("ratelimit: invalid retry-after")
)

// tokenScale is the fixed-point factor of the bucket state: one token equals
// tokenScale micro-units, so partial refills stay exact without floats.
const tokenScale int64 = 1_000_000

// Config configures a token bucket: one token refills every RefillInterval and
// the bucket holds at most Burst tokens.
type Config struct {
	// RefillInterval sets the time to refill one token.
	RefillInterval time.Duration
	// Burst sets the bucket capacity.
	Burst int
}

// validate checks the config; it is shared by all limiter constructors.
func (c Config) validate() error {
	if c.RefillInterval < time.Microsecond {
		return fmt.Errorf("%w: refill interval %s", ErrInvalidConfig, c.RefillInterval)
	}
	if c.Burst <= 0 {
		return fmt.Errorf("%w: burst %d", ErrInvalidConfig, c.Burst)
	}
	return nil
}

// Decision reports the limiter verdict.
type Decision struct {
	// Allowed reports whether a token was consumed.
	Allowed bool
	// RetryAfter is the wait until the next attempt when Allowed is false.
	RetryAfter time.Duration
	// Remaining is the number of whole tokens left after the attempt.
	Remaining int
}

// Limiter is a token bucket over arbitrary keys.
type Limiter interface {
	// Allow consumes one token from the bucket of key.
	Allow(ctx context.Context, key string) (Decision, error)
	// Pause empties the bucket of key and delays the refill by d, honoring a
	// provider Retry-After answer.
	Pause(ctx context.Context, key string, d time.Duration) error
}

// Metrics receives limiter events through an interface to keep the library free
// of metric clients; implementations must be safe for concurrent use.
type Metrics interface {
	// ObserveAllowed counts an allowed attempt.
	ObserveAllowed(key string, remaining int)
	// ObserveDenied counts a denied attempt and its retry delay.
	ObserveDenied(key string, retryAfter time.Duration)
	// ObserveError counts a limiter error; fallback reports whether the
	// local fallback served the request.
	ObserveError(key string, err error, fallback bool)
}

// validateKey rejects empty limiter keys.
func validateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w", ErrInvalidKey)
	}
	return nil
}

// retryAfter computes the wait until the next token for the local limiter:
// the missing part of a token converted to time, plus the remaining pause when
// the bucket baseline lies in the future.
func retryAfter(tokens int64, interval time.Duration, paused time.Duration) time.Duration {
	missing := tokenScale - tokens
	wait := time.Duration((missing*int64(interval) + tokenScale - 1) / tokenScale)
	return wait + paused
}
