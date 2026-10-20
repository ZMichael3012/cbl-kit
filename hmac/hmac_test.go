package hmac_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/hmac"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	secret := []byte("top-secret")
	body := []byte(`{"id":"evt_1"}`)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

	sig := hmac.Sign(secret, now, body)
	header := hmac.BuildHeader(hmac.Signature{Timestamp: now, Hex: sig})

	require.NoError(t, hmac.Verify(secret, header, body, now, hmac.DefaultWindow))
	require.NoError(t, hmac.Verify(secret, header, body, now.Add(4*time.Minute), hmac.DefaultWindow))

	require.ErrorIs(t, hmac.Verify(secret, header, []byte("tampered"), now, hmac.DefaultWindow), hmac.ErrSignatureMismatch)
	require.ErrorIs(t, hmac.Verify([]byte("other"), header, body, now, hmac.DefaultWindow), hmac.ErrSignatureMismatch)
}

func TestVerifyWindow(t *testing.T) {
	t.Parallel()

	secret := []byte("secret")
	body := []byte("body")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	header := hmac.BuildHeader(hmac.Signature{Timestamp: now, Hex: hmac.Sign(secret, now, body)})

	require.NoError(t, hmac.Verify(secret, header, body, now.Add(5*time.Minute), hmac.DefaultWindow))
	require.ErrorIs(t, hmac.Verify(secret, header, body, now.Add(5*time.Minute+time.Second), hmac.DefaultWindow), hmac.ErrExpired)
	require.ErrorIs(t, hmac.Verify(secret, header, body, now.Add(-5*time.Minute-time.Second), hmac.DefaultWindow), hmac.ErrExpired)
	require.ErrorIs(t, hmac.Verify(secret, header, body, now.Add(6*time.Minute), 0), hmac.ErrExpired)

	require.NoError(t, hmac.Verify(secret, header, body, now, 0))
}

func TestVerifyAnyRotation(t *testing.T) {
	t.Parallel()

	oldSecret := []byte("old")
	newSecret := []byte("new")
	body := []byte("payload")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

	header := hmac.BuildHeader(
		hmac.Signature{Timestamp: now, Hex: hmac.Sign(oldSecret, now, body)},
		hmac.Signature{Timestamp: now, Hex: hmac.Sign(newSecret, now, body)},
	)

	require.NoError(t, hmac.Verify(oldSecret, header, body, now, hmac.DefaultWindow))
	require.NoError(t, hmac.Verify(newSecret, header, body, now, hmac.DefaultWindow))
	require.NoError(t, hmac.VerifyAny([][]byte{[]byte("unknown"), newSecret}, header, body, now, hmac.DefaultWindow))
	require.ErrorIs(t, hmac.VerifyAny([][]byte{[]byte("unknown")}, header, body, now, hmac.DefaultWindow), hmac.ErrSignatureMismatch)
}

func TestParseHeaderMalformed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	validHex := hmac.Sign([]byte("s"), now, nil)

	tests := []struct {
		name   string
		header string
	}{
		{name: "empty", header: ""},
		{name: "no signature", header: "t=1760000000"},
		{name: "no timestamp", header: "v1=" + validHex},
		{name: "bad timestamp", header: "t=abc,v1=" + validHex},
		{name: "negative timestamp", header: "t=-5,v1=" + validHex},
		{name: "bad hex", header: "t=1760000000,v1=zz"},
		{name: "short signature", header: "t=1760000000,v1=00ff"},
		{name: "bad part", header: "t=1760000000,v1" + validHex},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := hmac.ParseHeader(tt.header)
			require.ErrorIs(t, err, hmac.ErrMalformed)
		})
	}

	ts, sigs, err := hmac.ParseHeader("unknown=1, t=1760000000, v1=" + validHex)
	require.NoError(t, err)
	assert.Equal(t, int64(1760000000), ts.Unix())
	assert.Len(t, sigs, 1)
}

func TestBuildHeaderFormat(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	header := hmac.BuildHeader(
		hmac.Signature{Timestamp: now, Hex: "aa"},
		hmac.Signature{Timestamp: now, Hex: "bb"},
	)
	assert.Equal(t, fmt.Sprintf("t=%d,v1=aa,v1=bb", now.Unix()), header)
	assert.True(t, strings.HasPrefix(header, "t="), header)
	assert.Equal(t, "", hmac.BuildHeader())
}
