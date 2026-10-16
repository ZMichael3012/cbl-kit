# cbl-kit

Shared Go library of the CBL platform: money type, HMAC signing, Gin and gRPC
middleware, transactional outbox, and idempotent inbox.

Repository rules and conventions: `AGENTS.md`, global rules `../../AGENTS.md`.
Task specs: `.specs/CBL-1.*.md`.

## Packages

| Package | Purpose |
| :-- | :-- |
| `money` | Money type int64 + decimals, float forbidden |
| `hmac` | Webhook signing over the raw body, replay window |
| `ginmw` | request_id, problem+json, access log, recovery |
| `grpcmw` | Interceptors and domain error mapping |
| `outbox` | Event write within a transaction, relay, kafkapub |
| `inbox` | Idempotency guard, retry/DLQ, kafkaconsumer |

## Commands

```powershell
go build ./...
go vet ./...
go test ./... -cover
go test -tags integration ./outbox/... ./inbox/...   # requires Docker (testcontainers)
```

## Migrations

`migrations/outbox.sql` and `migrations/inbox.sql` are reference DDL from
`.specs/CBL-003-contracts.md`, copied into services as goose migrations.
GORM `AutoMigrate` is forbidden.
