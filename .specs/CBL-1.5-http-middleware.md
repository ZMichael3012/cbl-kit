# CBL-1.5. HTTP middleware (Gin)

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `ginmw` |
| Size | S |
| Wave and parallel group | W0-KIT |
| Depends on | none |
| Contracts | error format RFC 9457 from `CBL-003`, section 1 |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Unified behavior of all HTTP services on Gin (Billing API, Back-office, API Gateway,
Webhook Ingress): request_id, access logging, errors in the format
`application/problem+json` (RFC 9457), panic protection.

## 2. Boundaries

In scope: middleware and error helpers for Gin.
Out of scope: authentication, rate limit, Prometheus metrics (tasks E7, 0.7).

## 3. Functional requirements

FR-1. `RequestID() gin.HandlerFunc`: takes `X-Request-Id` or generates a UUIDv7, puts it into
the context, returns it in the response header.

FR-2. `AccessLog(log *zap.Logger) gin.HandlerFunc`: logs the method, path, status, duration,
request_id; errors of level 5xx - error, 4xx - warn.

FR-3. Problem JSON:

```go
type Problem struct {
  Type, Title string; Status int; Code string; Detail string; RequestID string
}
func AbortWithProblem(c *gin.Context, status int, code, title, detail string)
func AbortFromError(c *gin.Context, err error) // mapping grpcmw.Kind -> HTTP status
```

FR-4. `Recovery(log *zap.Logger) gin.HandlerFunc`: panic -> 500 problem+json, stack to the log,
response without internal details.

FR-5. Error kind mapping: NotFound -> 404, InvalidArgument -> 422, AlreadyExists -> 409,
FailedPrecondition -> 409, ResourceExhausted -> 429, Unavailable -> 503, Internal -> 500.

## 4. Contracts

The error body and `request_id` - `CBL-003`, section 1. The `code` value is a machine-readable
string (for example, `idempotency_key_reuse`).

## 5. Data model and migrations

Not applicable.

## 6. State machines

Not applicable.

## 7. Edge cases

Panic in the handler, a write after `WriteHeader` (check `c.Writer.Written()`), empty
`X-Request-Id`, long headers, nested errors.

## 8. Non-functional requirements

Overhead not more than 30 microseconds per request; structured logging.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | httptest: request_id generation and propagation, problem+json body and header, recovery, mapping of all Kind, AccessLog |
| Integration | not applicable |
| E2E | not applicable |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.5: Gin middleware problem+json and request_id`; no commits in the current session.

## 11. Open questions

- None.
