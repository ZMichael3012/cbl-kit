package ratelimit

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfterSeconds bounds delta-seconds before the multiplication into a
// duration: larger values are clamped to the maximum duration, never wrapped.
const maxRetryAfterSeconds = int64(math.MaxInt64) / int64(time.Second)

// ParseRetryAfter parses an HTTP Retry-After header value: delta-seconds or an
// HTTP-date; a date in the past yields zero. Values too large for a duration are
// clamped to the maximum duration.
func ParseRetryAfter(value string, now time.Time) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("%w: empty value", ErrInvalidRetryAfter)
	}
	if seconds, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if seconds < 0 {
			return 0, fmt.Errorf("%w: %q", ErrInvalidRetryAfter, value)
		}
		if seconds > maxRetryAfterSeconds {
			return time.Duration(math.MaxInt64), nil
		}
		return time.Duration(seconds) * time.Second, nil
	}
	when, err := http.ParseTime(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidRetryAfter, value)
	}
	if !when.After(now) {
		return 0, nil
	}
	return when.Sub(now), nil
}

// FormatRetryAfter renders a duration as integer seconds rounded up, minimum 1,
// which is the always-valid Retry-After form for problem responses.
func FormatRetryAfter(d time.Duration) string {
	seconds := int64((d + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}
