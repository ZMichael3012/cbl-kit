// Package webhooksig implements provider webhook signature verification strategies.
// A strategy is selected per provider and verifies the request against a shared
// secret: raw HMAC (hex or base64), canonical JSON HMAC or token echo.
// Every comparison is constant-time; the canonical serializer is deterministic.
package webhooksig

import (
	"errors"
	"net/http"
)

// Package errors are returned wrapped through fmt.Errorf with %w.
var (
	// ErrMalformed reports a malformed signature, header, body or configuration.
	ErrMalformed = errors.New("webhooksig: malformed input")
	// ErrSignatureMismatch reports a digest that does not match the secret.
	ErrSignatureMismatch = errors.New("webhooksig: signature mismatch")
	// ErrEchoMismatch reports a token that does not match the secret.
	ErrEchoMismatch = errors.New("webhooksig: token mismatch")
	// ErrSignatureAbsent reports a missing signature header or token field.
	ErrSignatureAbsent = errors.New("webhooksig: signature absent")
)

// Algorithm selects the hash of an HMAC strategy. The zero value is SHA256.
type Algorithm int

const (
	// SHA256 selects HMAC-SHA256.
	SHA256 Algorithm = iota
	// SHA512 selects HMAC-SHA512.
	SHA512
)

// Encoding selects the wire encoding of the HMAC digest. The zero value is Hex.
type Encoding int

const (
	// Hex selects the lowercase hex encoding.
	Hex Encoding = iota
	// Base64 selects the standard base64 encoding.
	Base64
)

// Request carries the raw webhook request for verification.
type Request struct {
	// Header carries the HTTP headers; lookup is case-insensitive.
	Header http.Header
	// Body carries the raw request body as received.
	Body []byte
}

// Strategy verifies an inbound provider webhook against a shared secret.
type Strategy interface {
	// Verify returns nil when the request matches the secret; otherwise it
	// returns a wrapped ErrSignatureAbsent, ErrMalformed, ErrSignatureMismatch
	// or ErrEchoMismatch, which callers map to the signature state (CF-4).
	Verify(secret []byte, req Request) error
}
