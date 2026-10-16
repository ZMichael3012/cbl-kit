// Package ratelimit implements a token bucket rate limiter shared through Redis
// with an in-process fallback. The limiter carries no domain logic and uses no
// floats: tokens are counted in fixed-point micro-units, and elapsed time is
// converted exactly with integer arithmetic.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
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

// maxBurst bounds the bucket capacity: provider bursts are in the hundreds, the
// cap keeps the fixed-point arithmetic (burst * tokenScale) far from int64 limits.
const maxBurst = 1_000_000

// Config configures a token bucket: one token refills every RefillInterval and
// the bucket holds at most Burst tokens.
type Config struct {
	// RefillInterval sets the time to refill one token; it must be at least one
	// microsecond, the granularity shared with the Redis form.
	RefillInterval time.Duration
	// Burst sets the bucket capacity; it must be positive and at most maxBurst.
	Burst int
}

// validate checks the config; it is shared by all limiter constructors.
func (c Config) validate() error {
	if c.RefillInterval < time.Microsecond {
		return fmt.Errorf("%w: refill interval %s", ErrInvalidConfig, c.RefillInterval)
	}
	if c.Burst <= 0 || c.Burst > maxBurst {
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

// Limiter is a token bucket over arbitrary keys. Keys are logical identifiers
// chosen by the caller (for example provider:operation for the connector); the
// Redis form prefixes them with rl:provider: (CBL-003 section 7).
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
	wait := time.Duration(mulDivCeil(missing, int64(interval), tokenScale))
	return wait + paused
}

// mulDiv returns floor(a*b/d) for a, b >= 0 and d > 0 through 128-bit
// intermediate math, so large intervals cannot overflow int64. The callers keep
// the quotient below 2^64 (token arithmetic with a >= d or a < d bounded by
// tokenScale), so bits.Div64 never panics.
func mulDiv(a, b, d int64) int64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))   //nolint:gosec // G115: a and b are non-negative by the contract
	quotient, _ := bits.Div64(hi, lo, uint64(d)) //nolint:gosec // G115: d is positive by the contract
	return int64(quotient)                       //nolint:gosec // G115: the quotient fits int64 by the caller domain
}

// mulDivCeil returns ceil(a*b/d) with the same preconditions as mulDiv; the
// ceiling keeps the reported wait from ever understating the real one.
func mulDivCeil(a, b, d int64) int64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))           //nolint:gosec // G115: a and b are non-negative by the contract
	quotient, remainder := bits.Div64(hi, lo, uint64(d)) //nolint:gosec // G115: d is positive by the contract
	if remainder > 0 {
		quotient++
	}
	return int64(quotient) //nolint:gosec // G115: the quotient fits int64 by the caller domain
}
