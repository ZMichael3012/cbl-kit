// Package hmac implements signing and verification of CBL platform webhooks.
// The signature is computed over the raw request body, and the replay window is bounded.
package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultWindow sets the allowed clock skew window for signature timestamps.
const DefaultWindow = 5 * time.Minute

// Package errors are returned wrapped through fmt.Errorf with %w.
var (
	// ErrMalformed reports a malformed signature header.
	ErrMalformed = errors.New("hmac: malformed header")
	// ErrSignatureMismatch reports a signature mismatch.
	ErrSignatureMismatch = errors.New("hmac: signature mismatch")
	// ErrExpired reports a timestamp outside the replay window.
	ErrExpired = errors.New("hmac: timestamp outside replay window")
)

// Signature describes a single signature for building a header value.
type Signature struct {
	// Timestamp sets the signature time.
	Timestamp time.Time
	// Hex sets the hex representation of HMAC-SHA256.
	Hex string
}

// Sign computes the hex HMAC-SHA256(secret, "<unix>.<body>").
func Sign(secret []byte, timestamp time.Time, body []byte) string {
	return hex.EncodeToString(signRaw(secret, timestamp.Unix(), body))
}

// BuildHeader builds a header of the form t=<unix>,v1=<hex>[,v1=<hex>].
// All signatures use the timestamp of the first one.
func BuildHeader(signatures ...Signature) string {
	if len(signatures) == 0 {
		return ""
	}
	parts := make([]string, 0, len(signatures)+1)
	parts = append(parts, "t="+strconv.FormatInt(signatures[0].Timestamp.Unix(), 10))
	for _, s := range signatures {
		parts = append(parts, "v1="+s.Hex)
	}
	return strings.Join(parts, ",")
}

// ParseHeader parses the header into a timestamp and a list of signatures.
// Unknown parts are ignored for forward compatibility.
func ParseHeader(header string) (time.Time, [][]byte, error) {
	var ts int64 = -1
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			return time.Time{}, nil, fmt.Errorf("%w: %q", ErrMalformed, part)
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				return time.Time{}, nil, fmt.Errorf("%w: timestamp %q", ErrMalformed, value)
			}
			ts = parsed
		case "v1":
			raw, err := hex.DecodeString(value)
			if err != nil || len(raw) != sha256.Size {
				return time.Time{}, nil, fmt.Errorf("%w: signature %q", ErrMalformed, value)
			}
			sigs = append(sigs, raw)
		}
	}
	if ts < 0 {
		return time.Time{}, nil, fmt.Errorf("%w: timestamp missing", ErrMalformed)
	}
	if len(sigs) == 0 {
		return time.Time{}, nil, fmt.Errorf("%w: signatures missing", ErrMalformed)
	}
	return time.Unix(ts, 0).UTC(), sigs, nil
}

// Verify verifies the header signature for a single secret.
func Verify(secret []byte, header string, body []byte, now time.Time, window time.Duration) error {
	return VerifyAny([][]byte{secret}, header, body, now, window)
}

// VerifyAny verifies that one of the header signatures matches one of the secrets.
// Multiple secrets support secret rotation without downtime.
func VerifyAny(secrets [][]byte, header string, body []byte, now time.Time, window time.Duration) error {
	ts, sigs, err := ParseHeader(header)
	if err != nil {
		return err
	}
	if window <= 0 {
		window = DefaultWindow
	}
	delta := now.Sub(ts)
	if delta < 0 {
		delta = -delta
	}
	if delta > window {
		return fmt.Errorf("%w: delta %s", ErrExpired, delta)
	}
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		expected := signRaw(secret, ts.Unix(), body)
		for _, sig := range sigs {
			if hmac.Equal(expected, sig) {
				return nil
			}
		}
	}
	return ErrSignatureMismatch
}

// signRaw computes the raw HMAC-SHA256 per the platform rule.
func signRaw(secret []byte, unix int64, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(unix, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return mac.Sum(nil)
}
