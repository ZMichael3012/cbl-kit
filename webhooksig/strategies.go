package webhooksig

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

// RawHMAC verifies an HMAC digest computed over the raw request body.
type RawHMAC struct {
	// Header is the header carrying the digest.
	Header string
	// Algorithm selects the hash; the zero value is SHA256.
	Algorithm Algorithm
	// Encoding selects the digest encoding; the zero value is Hex.
	Encoding Encoding
	// Prefix is an optional digest prefix, for example "sha256=" of BTCPay;
	// when set, the header value must start with it.
	Prefix string
}

// Verify implements Strategy over the raw body.
func (s RawHMAC) Verify(secret []byte, req Request) error {
	hashFunc, err := hashFunc(s.Algorithm)
	if err != nil {
		return err
	}
	return verifyHMAC(secret, s.Header, s.Prefix, s.Encoding, hashFunc, req.Body, req)
}

// CanonicalJSONHMAC verifies an HMAC digest computed over the canonical JSON
// form of the request body (for example the NOWPayments x-nowpayments-sig).
type CanonicalJSONHMAC struct {
	// Header is the header carrying the digest.
	Header string
	// Algorithm selects the hash; the zero value is SHA256.
	Algorithm Algorithm
	// Encoding selects the digest encoding; the zero value is Hex.
	Encoding Encoding
}

// Verify implements Strategy over the canonical JSON form of the body.
func (s CanonicalJSONHMAC) Verify(secret []byte, req Request) error {
	hashFunc, err := hashFunc(s.Algorithm)
	if err != nil {
		return err
	}
	canonical, err := CanonicalJSON(req.Body)
	if err != nil {
		return err
	}
	return verifyHMAC(secret, s.Header, "", s.Encoding, hashFunc, canonical, req)
}

// TokenEcho verifies a token echo: the body field value is compared with the
// secret in constant time (CoinGate; no HMAC is involved).
type TokenEcho struct {
	// Field is the body field carrying the token; empty means "token".
	Field string
}

// Verify implements Strategy by comparing the body token with the secret.
// The body must be exactly one JSON object; duplicate keys and trailing data
// are malformed, matching the strict canonical decoder.
func (s TokenEcho) Verify(secret []byte, req Request) error {
	if len(secret) == 0 {
		return fmt.Errorf("%w: empty secret", ErrMalformed)
	}
	field := s.Field
	if field == "" {
		field = "token"
	}
	decoded, err := decodeStrict(req.Body)
	if err != nil {
		return err
	}
	body, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: body is not a JSON object", ErrMalformed)
	}
	value, ok := body[field]
	if !ok {
		return fmt.Errorf("%w: field %q", ErrSignatureAbsent, field)
	}
	token, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: field %q is not a string", ErrMalformed, field)
	}
	if subtle.ConstantTimeCompare([]byte(token), secret) != 1 {
		return ErrEchoMismatch
	}
	return nil
}

// verifyHMAC checks the header digest of signed against the secret.
func verifyHMAC(secret []byte, headerName, prefix string, encoding Encoding, hashFunc func() hash.Hash, signed []byte, req Request) error {
	if len(secret) == 0 {
		return fmt.Errorf("%w: empty secret", ErrMalformed)
	}
	if headerName == "" {
		return fmt.Errorf("%w: empty header name", ErrMalformed)
	}
	value := req.Header.Get(headerName)
	if value == "" {
		return fmt.Errorf("%w: header %q", ErrSignatureAbsent, headerName)
	}
	if prefix != "" {
		trimmed, ok := strings.CutPrefix(value, prefix)
		if !ok {
			return fmt.Errorf("%w: header %q misses prefix %q", ErrMalformed, headerName, prefix)
		}
		value = trimmed
	}
	actual, err := decodeDigest(value, encoding)
	if err != nil {
		return err
	}
	mac := hmac.New(hashFunc, secret)
	mac.Write(signed)
	if !hmac.Equal(mac.Sum(nil), actual) {
		return ErrSignatureMismatch
	}
	return nil
}

// hashFunc resolves the hash constructor of an algorithm.
func hashFunc(algorithm Algorithm) (func() hash.Hash, error) {
	switch algorithm {
	case SHA256:
		return sha256.New, nil
	case SHA512:
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("%w: unknown algorithm %d", ErrMalformed, algorithm)
	}
}

// decodeDigest decodes the wire digest of an encoding.
func decodeDigest(value string, encoding Encoding) ([]byte, error) {
	switch encoding {
	case Hex:
		decoded, err := hex.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("%w: digest %q", ErrMalformed, truncate(value))
		}
		return decoded, nil
	case Base64:
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("%w: digest %q", ErrMalformed, truncate(value))
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("%w: unknown encoding %d", ErrMalformed, encoding)
	}
}
