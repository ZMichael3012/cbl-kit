# CBL-1.6. HMAC signature and replay protection

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `hmac` |
| Size | S |
| Wave and parallel group | W0-KIT |
| Depends on | none |
| Contracts | signature format from `CBL-003`, section 4 |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Signing of webhooks in both directions: incoming from the gateway (Webhook Ingress) and outgoing
to merchants (Notification). The signature is computed over the raw body, the replay window is
limited.

## 2. Boundaries

In scope: signature computation and verification, header parsing, replay window, secret rotation.
Out of scope: registration of webhook endpoints and secret storage (task 8.1), URL protection
(SSRF, 8.2).

## 3. Functional requirements

FR-1. Header format: `t=<unix>,v1=<hex>[,v1=<hex>]` (multiple v1 for rotation).

FR-2. API:

```go
const DefaultWindow = 5 * time.Minute
func Sign(secret []byte, timestamp time.Time, body []byte) string // hex HMAC-SHA256(secret, t + "." + body)
func BuildHeader(secrets ...Signature) string
func Verify(secret []byte, header string, body []byte, now time.Time, window time.Duration) error
func VerifyAny(secrets [][]byte, header string, body []byte, now time.Time, window time.Duration) error
func ParseHeader(header string) (timestamp time.Time, signatures [][]byte, err error)
```

FR-3. The signature is computed over `strconv.FormatInt(t, 10) + "." + body`; the body must not
be re-encoded. Verification: a match of at least one signature from `v1` (constant-time
comparison `hmac.Equal`), age `|now - t| <= window`, otherwise an error.

FR-4. Errors: `ErrMalformed`, `ErrSignatureMismatch`, `ErrExpired` (sentinel); wrapped
via `fmt.Errorf` with `%w`.

## 4. Contracts

Compatible with `X-Billing-Signature` from `CBL-003`, section 4, and with provider webhook
verification (provider headers are mapped by the service into this format).

## 5. Data model and migrations

Not applicable.

## 6. State machines

Not applicable.

## 7. Edge cases

Empty body, missing `t` or `v1`, multiple signatures, timestamp in the future, exactly at the
window boundary, non-hex signature, empty secret (configuration error), very long body (no
length limit, the limit is at the HTTP server level).

## 8. Non-functional requirements

Constant-time comparison; HMAC-SHA256 computation on bodies up to 1 MB not more than 5 ms.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | Sign/Verify round-trip, body change, expired/future ts, rotation (2 signatures), malformed, window boundary |
| Integration | not applicable |
| E2E | not applicable |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.6: HMAC signature with replay protection`; no commits in the current session.

## 11. Open questions

- None.
