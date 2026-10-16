# CBL-1.3. inbox package and idempotent consumer

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `inbox` |
| Size | L |
| Wave and parallel group | W0-KIT |
| Depends on | 1.2, 1.7 (CF-1) |
| Contracts | inbox DDL and the retry/DLQ rules from `CBL-003`, section 6 |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Kafka provides at-least-once: every consumer must be idempotent. The inbox table
`(consumer, event_id)` in the same transaction as the processing effect cuts off duplicates;
errors go to retry topics and the DLQ (`CBL-003`, section 6).

## 2. Boundaries

In scope: the idempotency Guard, the message-processing Processor, retry/DLQ routing.
Out of scope: configuration of franz-go consumer groups in services, lag monitoring (0.7).

## 3. Functional requirements

FR-1. GORM model `Record` (table `inbox`): `consumer`, `event_id`, `processed_at`, a composite
primary key `(consumer, event_id)`. Reference migration `migrations/inbox.sql`.

FR-2. Guard:

```go
type Guard struct{}
func NewGuard() *Guard
// Process executes fn in the same transaction as the inbox record insert.
// Returns processed=false without calling fn if the event has already been processed.
func (g *Guard) Process(ctx context.Context, db *gorm.DB, consumer, eventID string, fn func(tx *gorm.DB) error) (processed bool, err error)
```

FR-3. The inbox insert is performed with `INSERT ... ON CONFLICT DO NOTHING`;
`RowsAffected == 0` means a duplicate; an `fn` error rolls back the entire transaction,
including the inbox record.

FR-4. Processor:

```go
type Message struct{ Topic, Key string; Headers map[string]string; Value []byte }
type Handler interface{ Handle(ctx context.Context, db *gorm.DB, msg Message) error }
type Router struct{} // retry/DLQ routing
func NewRouter(mainTopic string, maxAttempts int) *Router
func (r *Router) Next(topic string, attempt int, err error) (string, bool) // topic, isDLQ
type Processor struct{}
func NewProcessor(db *gorm.DB, guard *Guard, handler Handler, pub outbox.Publisher, router *Router, log *zap.Logger) *Processor
func (p *Processor) Process(ctx context.Context, msg Message) error
```

FR-5. Retry rules: the `x-retry-attempt` counter in the headers; on a processing error the
message is published to `<topic>.retry.5s` -> `.retry.1m` -> `.retry.10m` -> `<topic>.dlq`
(the reason is in the `x-error` header); `event_id` is preserved so that the inbox logic works
on a retry. When attempts are exhausted - the DLQ and acknowledgment of the original message.

FR-6. The thin `kafkaconsumer` adapter (franz-go): `Run(ctx, topics, groupID, batchSize)`
reads messages and passes them to the `Processor`; the offset is committed after successful
processing or the DLQ.

## 4. Contracts

DDL and rules from `CBL-003` (section 6). Headers: `event_id`, `event_type`, `traceparent`,
`x-retry-attempt`, `x-error`.

## 5. Data model and migrations

`migrations/inbox.sql` from `CBL-003`.

## 6. State machines

Attempt routing: `main -> retry.5s -> retry.1m -> retry.10m -> dlq`.

## 7. Edge cases

A duplicate before and after processing, a handler error after a side effect (the effect and
the inbox are in the same transaction), a lost `event_id`, out-of-order messages, empty
headers, concurrent processing of the same event_id by different instances (unique key).

## 8. Non-functional requirements

A duplicate does not cause a repeated effect. Processing of a single message happens in a
single DB transaction. Unit tests without Docker; integration (testcontainers) under the
`integration` tag.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | Guard (insert, duplicate, rollback), Router (all transitions, DLQ), Processor (success, error, duplicate) |
| Integration | real PostgreSQL: concurrent Process calls for the same event_id; the effect + inbox are atomic |
| E2E | not applicable |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.3: inbox and idempotent consumer with retry/DLQ`; no commits in the current session.

## 11. Open questions

- The retry delay is implemented through separate topics; the exact wait timer is configured
  by the service consumer, the package defines only the route.
