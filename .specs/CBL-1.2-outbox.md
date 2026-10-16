# CBL-1.2. outbox package

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `outbox` (+ `kafkapub` adapter) |
| Size | L |
| Wave and parallel group | W0-KIT |
| Depends on | 1.7 (CF-1, for the Kafka envelope), 1.1 |
| Contracts | outbox DDL from `CBL-003`, section 6; event headers from `envelope.proto` |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Outbox guarantees atomicity of the business transaction and the event (`CBL-000`, rationale
in section 10): the event record goes in the same transaction as the business data, and the
relay publishes to Kafka at-least-once. The package is used by all producer services (Payment,
Ledger, Payout, Webhook Ingress, Reconciler).

## 2. Boundaries

In scope: the model and transactional recording of an event, relay with `FOR UPDATE SKIP
LOCKED`, metrics via an interface, the franz-go publish adapter, the reference SQL migration.
Out of scope: event consumption (1.3), service-specific topics, Debezium CDC.

## 3. Functional requirements

FR-1. GORM model `Record` (table `outbox`): `id int64`, `event_id uuid unique`, `topic`,
`key`, `headers jsonb`, `payload bytea`, `created_at`, nullable `published_at`. Automatic
table creation is forbidden: the package ships the file `migrations/outbox.sql` (goose) for
copying into a service.

FR-2. API:

```go
type Envelope struct{ EventID, Topic, Key string; Headers map[string]string; Payload []byte }
func NewEnvelope(eventType, topic, key string, headers map[string]string, payload []byte) (Envelope, error) // event_id = uuidv7
func (e Envelope) Validate() error

type Store struct{}
func NewStore() *Store
func (s *Store) Append(ctx context.Context, tx *gorm.DB, env Envelope) error

type Publisher interface{ Publish(ctx context.Context, env Envelope) error }
type Metrics interface{ AddPublished(n int, topic string); SetPending(n int, topic string) } // default noop

type Relay struct{}
func NewRelay(db *gorm.DB, pub Publisher, log *zap.Logger, opts ...Option) *Relay
func (r *Relay) Run(ctx context.Context) error       // loop with an interval
func (r *Relay) PublishBatch(ctx context.Context) (int, error) // testable unit
```

FR-3. `PublishBatch`: within a transaction, select a batch of unpublished records (`ORDER BY
id LIMIT n FOR UPDATE SKIP LOCKED`), publish them, set `published_at`; a publish error -
rollback, the event remains for the next loop; batches are processed until the table is
drained.

FR-4. `kafkapub` adapter: a `Publisher` on top of `franz-go` (`kgo.Client.ProduceSync`), an
idempotent producer (`kgo.RequiredAcks(kgo.AllISRAcks())`, `kgo.ProducerBatchMaxBytes`,
retries); event headers are carried over to Kafka headers; the partition key is
`Envelope.Key`.

FR-5. Metrics: the `Metrics` interface (outbox lag, number published), a noop implementation
by default; the Prometheus adapter is connected in 0.7.

## 4. Contracts

DDL strictly from `CBL-003` (section 6). Headers: `event_id`, `event_type`, `occurred_at`,
`producer`, `traceparent`, `schema_version` (`envelope.proto`). Errors are introduced as
sentinel errors, panics are forbidden.

## 5. Data model and migrations

`migrations/outbox.sql`: `CREATE TABLE outbox (...)` from `CBL-003` plus a partial
`published_at` index (`WHERE published_at IS NULL`) for efficient polling.

## 6. State machines

Not applicable.

## 7. Edge cases

Concurrent relay instances (`SKIP LOCKED`), a crash after publishing and before
`published_at` (at-least-once, a duplicate is acceptable, the consumer is idempotent), an
empty batch, Kafka errors, an invalid envelope, a very large payload.

## 8. Non-functional requirements

The publish lag under normal operation is at most 1 second (default interval 250 ms, batch
100). The relay does not block business transactions. Unit tests without Docker; integration
tests (testcontainers PostgreSQL and Kafka) under the `integration` tag.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | envelope validation, headers, topic breakdown, option configuration |
| Integration | a write in a transaction and a transaction rollback leave no outbox record; concurrent relays; re-publishing after a failure |
| E2E | not applicable (verified in services) |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.2: outbox with a relay based on SKIP LOCKED`; no commits in the current session.

## 11. Open questions

- The CDC option (Debezium) is recorded as an alternative and is not implemented.
