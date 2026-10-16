# AGENTS.md - cbl-kit

Shared Go library of the CBL platform: money type, HMAC, Gin and gRPC middleware, outbox and inbox.
Mandatory global rules: `../../AGENTS.md`, `../../.specs/CBL-001-development-process.md`,
`../../.specs/CBL-002-repos-and-docker.md` (section 6, libraries).
The em dash (U+2014) is forbidden, only the hyphen is used.

## 1. Purpose and boundaries

In scope: reusable packages without domain logic, reference outbox/inbox migrations, tests.

Out of scope: business logic of services, concrete topics and event schemas (in services and
`cbl-protos`), Prometheus metric adapters (task 0.7).

## 2. Packages

| Package | Task | Consumers |
| :-- | :-- | :-- |
| `money` | CBL-1.1 | all services |
| `hmac` | CBL-1.6 | `cbl-webhook-ingress`, `cbl-notification` |
| `ginmw` | CBL-1.5 | HTTP services |
| `grpcmw` | CBL-1.4 | gRPC services |
| `outbox` (+ `kafkapub`) | CBL-1.2 | Payment, Ledger, Payout, Webhook Ingress, Reconciler |
| `inbox` (+ `kafkaconsumer`) | CBL-1.3 | Payment, Ledger, Payout, Connector, Notification |

## 3. Architecture and dependencies

```
cbl-kit/
├── money/          # int64 minor units + decimals, no float
├── hmac/           # signature over the raw body, replay window
├── ginmw/          # request_id, problem+json, access log, recovery
├── grpcmw/         # errors, recovery, metadata, request_id
├── outbox/         # Store, Envelope, Relay, kafkapub
├── inbox/          # Guard, Processor, kafkaconsumer
└── migrations/     # reference SQL for services
```

Dependencies: `gorm.io/gorm`, `gorm.io/driver/postgres`, `gin-gonic/gin`, `go.uber.org/zap`,
`google.golang.org/grpc`, `google.golang.org/protobuf`, `google/uuid`, `twmb/franz-go`,
`stretchr/testify`, `testcontainers-go` (integration tests only).

Rules: `AutoMigrate` is forbidden; migrations in services run via goose, reference files are
copied from this repository; panics in business code are forbidden; errors are wrapped with `%w`;
float is forbidden.

## 4. Commands

```powershell
go build ./...
go vet ./...
go test ./... -cover
go test -tags integration ./outbox/... ./inbox/...   # requires Docker (testcontainers)
docker run --rm -v "%CD%":/app -w /app golangci/golangci-lint:v1.64.8-alpine golangci-lint run ./...
powershell -File ../../scripts/check-no-emdash.ps1 -Root .
```

cbl-kit is a library: it has no local e2e stand and no PowerShell run scripts; integration checks run with `make test-integration` here and in every service repository that depends on kit (their scenarios follow their `test/e2e/E2E.md` instructions).

## 5. Development rules

- Pipeline: spec in `.specs/` -> tests (unit; integration under the `integration` tag) -> code.
- Unit+integration coverage is at least 80% per repository, for `money` at least 95%.
- Godoc docstrings are mandatory in English: the first line starts with the entity name and ends with a period; comments explain why; errors are wrapped with `%w`; panics in business code are forbidden.
- Comments explain why; exported entity names have no abbreviations.
- Commit: `CBL-<epic>.<number>: <short message>`, the spec is in the same commit.

## 6. Trade-offs

| ID | Decision | Pros | Cons and compensation |
| :-- | :-- | :-- | :-- |
| T-KK1 | `outbox.Publisher` interface, franz-go only in the `kafkapub` adapter | testability, services do not depend on the Kafka client directly | one extra layer; the adapter is thin and covered by unit tests |
| T-KK2 | reference SQL migrations instead of built-in AutoMigrate | the service owns the schema, history in goose | DDL duplication in services; files are copied as is |
| T-KK3 | metrics via an interface, without a Prometheus dependency | the library stays free of heavy dependencies | services need an adapter; added in 0.7 |
| T-KK4 | kafkaconsumer is thin, logic in Processor | the core is covered by unit tests without a broker | integration is verified in services |
| T-KK5 | Kafka publication inside the open relay transaction | simplicity, minimal lag | a network call holds row locks; the metric is inflated on rollback | a batch of at most 100, revisit to a two-phase scheme based on the results of 2.9 and 14.2 |
| T-KK6 | `storagex.Migrate` extracted into kit | services do not duplicate goose wiring | kit gains a goose dependency | the dependency is only for the migration runtime, not used in business code |
| T-KK7 | Main wiring (health, metrics stub, getenv) is duplicated in services | the service is autonomous and simple | duplication across 13+ repositories | extraction to cbl-kit/httpx after 0.7, when real metrics appear |

## 7. Known issues

| ID | Problem | Impact | Plan | Status |
| :-- | :-- | :-- | :-- | :-- |
| KI-K1 | Integration tests require Docker | without Docker there is no relay concurrency check | tag `integration`, CI in 0.2 | open |
| KI-K2 | outbox Kafka integration is not covered by default | the adapter is verified by unit tests | Kafka container in CI by task 0.2 | open |
| KI-K3 | The AddPublished metric is recorded before the relay transaction commit | possible inflation on publication rollback | move accounting after commit as part of 2.9 | open |

## 8. Task journal

| CBL | Date | Status | Spec | Commit |
| :-- | :-- | :-- | :-- | :-- |
| CBL-1.1 - 1.6 | 2026-10-14 | done, W1 review closed (unit+integration: outbox 82.8%, inbox 85.0%, money 96.3%; DLQ for poison messages, storagex, Kafka delivery timeout) | `.specs/CBL-1.*.md` | awaiting commit |
| CBL-1.3 (fix) | 2026-10-14 | done, findings of review 2.7: DLQ fallbacks for poison messages (an invalid event_id is sanitized into `x-original-event-id`; a missing event_type -> `unknown`; a corrupted retry envelope goes to DLQ instead of looping the partition) | `inbox/processor.go`, inbox tests | `cc5d796` |
| CBL-16.15 | 2026-10-16 | done (English comments and documentation across the library: code, tests, specs, AGENTS.md, README, migrations; the library has no local e2e stand; no code changes) | `.specs/CBL-16.15-conventions.md` | this commit |

## 9. Runbook

Diagnosing outbox lag:

1. The pending metric (Prometheus after 0.7) and relay logs: `published batch`, `publish failed`.
2. Check Kafka availability (`KAFKA_BROKERS`), producer permissions, batch size.
3. On a publication error events remain in the table and are resent by the next cycle;
   loss is impossible, duplicates are possible (at-least-once), consumers are idempotent via the inbox.
