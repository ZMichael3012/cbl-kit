package ratelimit_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/ratelimit"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", value: "5", want: 5 * time.Second},
		{name: "zero", value: "0", want: 0},
		{name: "padded", value: " 7 ", want: 7 * time.Second},
		{name: "http date future", value: "Fri, 16 Oct 2026 12:00:02 GMT", want: 2 * time.Second},
		{name: "http date past", value: "Fri, 16 Oct 2026 11:59:59 GMT", want: 0},
		{name: "negative", value: "-1", wantErr: true},
		{name: "fraction", value: "1.5", wantErr: true},
		{name: "garbage", value: "abc", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "huge seconds clamp", value: "9223372037", want: time.Duration(math.MaxInt64)},
		{name: "absurd seconds clamp", value: "18446744074", want: time.Duration(math.MaxInt64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ratelimit.ParseRetryAfter(tt.value, now)
			if tt.wantErr {
				require.ErrorIs(t, err, ratelimit.ErrInvalidRetryAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFormatRetryAfter(t *testing.T) {
	t.Parallel()
	require.Equal(t, "5", ratelimit.FormatRetryAfter(5*time.Second))
	require.Equal(t, "2", ratelimit.FormatRetryAfter(1500*time.Millisecond))
	require.Equal(t, "1", ratelimit.FormatRetryAfter(0))
	require.Equal(t, "1", ratelimit.FormatRetryAfter(-time.Second))
}
