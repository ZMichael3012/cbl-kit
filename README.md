# cbl-kit

Shared Go library of the CBL platform: money type, HMAC signing, provider webhook
verification, rate limiting, Gin and gRPC middleware, transactional outbox, and
idempotent inbox.

Repository rules and conventions: `AGENTS.md`, global rules `../../AGENTS.md`.
Task specs: `.specs/CBL-1.*.md`, `.specs/CBL-17.3-kit-primitives.md`,
`.specs/CBL-17.21-asset-catalog.md`.

## Packages

| Package | Purpose |
| :-- | :-- |
| `money` | Money type int64 + decimals, float forbidden, float-free provider parsing |
| `hmac` | Outbound webhook signing over the raw body, replay window |
| `webhooksig` | Inbound provider verification: raw HMAC, canonical JSON HMAC, token echo |
| `ratelimit` | Redis token bucket with a local fallback and Retry-After helpers |
| `ginmw` | request_id, problem+json, access log, recovery |
| `grpcmw` | Interceptors and domain error mapping |
| `outbox` | Event write within a transaction, relay, kafkapub |
| `inbox` | Idempotency guard, retry/DLQ, kafkaconsumer |
| `catalog` | Asset and network catalog parser, validation and provider-code lookups |

## Commands

```powershell
go build ./...
go vet ./...
go test ./... -cover
go test -tags integration ./outbox/... ./inbox/... ./ratelimit/...   # requires Docker (testcontainers)
```

## Migrations

`migrations/outbox.sql` and `migrations/inbox.sql` are reference DDL from
`.specs/CBL-003-contracts.md`, copied into services as goose migrations.
GORM `AutoMigrate` is forbidden.
