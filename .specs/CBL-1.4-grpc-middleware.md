# CBL-1.4. gRPC middleware

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `grpcmw` |
| Size | M |
| Wave and parallel group | W0-KIT |
| Depends on | 1.7 (CF-1) |
| Contracts | error mapping and deadlines from `CBL-003`, section 5 |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Unified interceptors for all gRPC services: request identifier, structured
logging, panic protection, mapping of domain errors to gRPC codes, propagation of
`request_id` and `traceparent`, extraction of `x-merchant-id` after mTLS.

## 2. Boundaries

In scope: server and client unary/stream interceptors, definition of domain error types.
Out of scope: specific domain errors of services, configuration of TLS credentials (in services).

## 3. Functional requirements

FR-1. The package defines `errorsx`-compatible error kinds (inside `grpcmw`):

```go
type Kind uint8 // NotFound, InvalidArgument, AlreadyExists, FailedPrecondition, ResourceExhausted, Unavailable, Internal
type Error struct{ Kind Kind; Code string; Message string; Err error }
func E(kind Kind, code, message string, err error) *Error
func KindOf(err error) Kind
```

FR-2. `UnaryServer(log *zap.Logger) grpc.UnaryServerInterceptor` combines: recovery
(panic -> Internal, stack logging, the panic does not escape), request_id (from metadata or
a new UUIDv7, placed into response headers), logging (method, code, duration),
error mapping.

FR-3. `MapError(err error) error`: `Error` -> `status.Error` with the code
(`NotFound->codes.NotFound`, `InvalidArgument->InvalidArgument`, `AlreadyExists->AlreadyExists`,
`FailedPrecondition->FailedPrecondition`, `ResourceExhausted->ResourceExhausted`,
`Unavailable->Unavailable`, `Internal->Internal`); unknown error -> `Internal` with the
message "internal error" without leaking details.

FR-4. `UnaryClient(log *zap.Logger) grpc.UnaryClientInterceptor`: propagation of `request_id`
and `traceparent` into metadata, logging of outgoing calls and errors; it does not set extra
deadlines.

FR-5. `MerchantID(ctx) (string, error)`: reads `x-merchant-id` from incoming metadata; an empty
value - `InvalidArgument`. Helper `RequestID(ctx) string`.

FR-6. Stream versions of the interceptors `StreamServer`, `StreamClient` with the same rules.

FR-7. Tracing: the package does not duplicate `otelgrpc`; the README states that
`otelgrpc.NewServerHandler()` must be connected separately.

## 4. Contracts

Code mapping and the deadline budget rule - `CBL-003`, section 5. Metadata: `x-merchant-id`,
`request_id`, `traceparent`.

## 5. Data model and migrations

Not applicable.

## 6. State machines

Not applicable.

## 7. Edge cases

Handler panic, error without Kind (Internal), nested errors via `%w`, missing
metadata, invalid request_id format (regeneration), concurrent calls.

## 8. Non-functional requirements

Interceptor overhead not more than 50 microseconds per call. Unit tests via `bufconn` without
network.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | bufconn server: success, each Kind -> code, panic, request_id propagation, MerchantID |
| Integration | not applicable |
| E2E | not applicable |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.4: gRPC middleware with error mapping`; no commits in the current session.

## 11. Open questions

- The client deadline budget is left to the services: values are taken from `CBL-003`, section 5.
