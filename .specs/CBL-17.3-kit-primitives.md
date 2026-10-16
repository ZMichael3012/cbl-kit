# CBL-17.3. Kit primitives: float-free amounts, webhook signature strategies, Redis token bucket

| Field | Value |
| :-- | :-- |
| Epic | E17 (task CBL-17.3) |
| Repository | `cbl-kit` |
| Size | S |
| Wave and parallel group | W5-PROVIDER, stage A |
| Depends on | CF-4 (`cbl-protos@cf-4`) for the `signature_state` semantics; provider plan section 15, items 1 - 3 |
| Contract version | none (Go library; consumers use the local `replace` in this workspace) |
| Status | done |

## 1. Context and goal

E17 (provider integrations: NOWPayments, CoinGate, BitPay, BTCPay) starts each adapter and the
webhook ingress with the same three primitives (plan section 15, items 1 - 3):

1. amounts from provider payloads must be parsed without `float64` (NOWPayments sends JSON
   numbers, others send decimal strings; `json.Number` keeps the literal text);
2. webhook verification strategies per provider (raw HMAC hex or base64, canonical JSON HMAC,
   token echo) with a deterministic canonical JSON serializer;
3. a per-provider token bucket in Redis (shared by connector replicas) with `Retry-After`
   support.

This task adds the `money` parsing extensions and the new `webhooksig` and `ratelimit`
packages. The techniques validate their inputs at the boundary (17.2, 17.4); the kit stays
free of domain logic.

## 2. Requirements

FR-1. `money` (extend the existing package, additive):

- `ParseJSONNumber(number json.Number, asset string, decimals uint32) (Money, error)`: exact
  parsing of a JSON number literal; the exponent form is allowed and handled by decimal point
  shifting (no float). A value that needs more than `decimals` fractional digits is an error;
  overflow is `ErrOverflow`.
- `ParseAny(value any, asset string, decimals uint32) (Money, error)`: accepts `string` and
  `json.Number`; rejects `float64`/`float32` with the new sentinel `ErrFloatForbidden` (the
  decoder must use `json.Decoder.UseNumber`); other types are `ErrInvalidAmount`.
- Existing `Parse`, `String`, arithmetics stay unchanged; the `money` package keeps coverage
  at least 95%.

FR-2. `webhooksig` (new package):

- `Algorithm` (`SHA256` default, `SHA512`) and `Encoding` (`Hex` default, `Base64`).
- `Request{Header http.Header; Body []byte}`.
- `Strategy` interface: `Verify(secret []byte, req Request) error`.
- `RawHMAC{Header, Algorithm, Encoding, Prefix}`: HMAC over the raw body; `Prefix` is stripped
  when set (`BTCPay-Sig: sha256=...`); a missing header/signature is `ErrSignatureAbsent`,
  malformed values are `ErrMalformed`, a mismatch is `ErrSignatureMismatch`.
- `CanonicalJSONHMAC{Header, Algorithm, Encoding}`: HMAC over the canonical JSON form of the
  body (NOWPayments `x-nowpayments-sig`).
- `TokenEcho{Field}` (default field `token`): constant-time comparison of the secret with the
  body field (CoinGate); a mismatch is `ErrEchoMismatch`, a missing/non-string field or an
  empty secret is `ErrMalformed`.
- `CanonicalJSON(raw []byte) ([]byte, error)`: deterministic compact JSON, object keys sorted,
  HTML escaping off, number literals preserved as `json.Number`; shared by
  `CanonicalJSONHMAC` and available to tests and mock profiles (17.5).
- `Registry`: `Register(provider, strategy)` (empty provider, nil strategy or a duplicate is an
  error) and `Lookup(provider) (Strategy, bool)`.
- All digest comparisons use `hmac.Equal` or `subtle.ConstantTimeCompare`.

FR-3. `ratelimit` (new package):

- `Config{RefillInterval time.Duration; Burst int}`: one token per `RefillInterval`, capacity
  `Burst`; validation errors for non-positive values. No floats anywhere (tokens are counted in
  internal integer micro-units).
- `Limiter` interface: `Allow(ctx, key) (Decision, error)` and `Pause(ctx, key, d) error`
  (honors a provider `Retry-After`); `Decision{Allowed bool; RetryAfter time.Duration;
  Remaining int}`.
- `Local`: in-process token bucket (mutex), injectable clock (`NewLocalWithClock`), used as a
  fallback and in tests.
- `Redis`: shared bucket implemented as an atomic Lua script over `HSET`/`HMGET`/`PEXPIRE`
  (no `TIME`; the caller clock is passed as an argument), constructor over `redis.Cmdable`
  (go-redis v9); the key namespace is `rl:provider:<key>`; a full-refill TTL keeps idle buckets
  from leaking.
- `WithFallback{primary, fallback}`: on a primary error switches to the fallback and reports
  through `Metrics`; the fallback is per-process and must be configured conservatively.
- `Metrics` interface (allowed, denied, error with the fallback flag); a nil metrics value is
  safe.
- `ParseRetryAfter(value string, now time.Time) (time.Duration, error)`: delta-seconds or
  HTTP-date; `FormatRetryAfter(d)` renders integer seconds rounded up, minimum 1.

FR-4. Quality gates: unit tests for every requirement (table-driven, boundary cases: exponent
forms, overflow, negative amounts, wrong digests, reordered JSON keys, HTML escapes, burst
exhaustion, pause windows, fallback switching); Redis semantics verified with miniredis in unit
tests and with a real Redis container under the `integration` tag; repository coverage at
least 80% (`money` at least 95%); `go build`, `go vet`, golangci-lint and the em dash check
green.

Dependency note: the `ratelimit` package adds `redis/go-redis/v9` v9.23.0 at runtime;
`alicebob/miniredis/v2` v2.40.0 (unit tests) and `testcontainers-go/modules/redis` v0.34.0
(integration tag). go-redis v9.23.0 forces `klauspost/cpuid/v2` v2.4.0 and
`go.uber.org/atomic` v1.12.0, which upgrades `stretchr/testify` to v1.12.1; govulncheck
reports no reachable vulnerabilities after the upgrade.

FR-5. Documentation: AGENTS.md packages table, dependency list, trade-offs (`T-KK8` fixed-point
token bucket without floats, `T-KK9` fallback limiter semantics) and the task journal; the
umbrella records the CBL-003 section 7 key correction (`rl:provider:<provider>:<op>`, no
window suffix in a token bucket) on instruction.

Out of scope: the HTTP resilience client extraction (17.2), provider manifests and the asset
catalog (17.20 - 17.22), TOTP and ECDSA signing (17.6, 17.8), the ingress response policy
(17.4), metric adapters (0.7).

## 3. Work plan

Spec -> tests first (money, webhooksig, ratelimit) -> implementation -> go build/vet, unit and
integration tests with coverage, golangci-lint, em dash check -> commit
`CBL-17.3: kit primitives for providers` (spec in the same commit) -> umbrella CBL-003 note.

Consumers after this task: `cbl-connector` (17.2 adapters and limiter), `cbl-webhook-ingress`
(17.4 strategies), `cbl-mock-gateway` (17.5 profiles), reconciler/payment amount parsing
(17.10, 17.13).
