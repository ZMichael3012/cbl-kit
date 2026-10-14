# CBL-1.2. Пакет outbox

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `outbox` (+ адаптер `kafkapub`) |
| Размер | L |
| Волна и параллельная группа | W0-KIT |
| Зависит от | 1.7 (CF-1, для Kafka-конверта), 1.1 |
| Контракты | outbox DDL из `CBL-003`, раздел 6; заголовки события из `envelope.proto` |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Outbox гарантирует атомарность бизнес-транзакции и события (`CBL-000`, обоснование в разделе 10):
запись события идет в ту же транзакцию, что и бизнес-данные, а relay публикует в Kafka
at-least-once. Пакет используется всеми сервисами-продюсерами (Payment, Ledger, Payout,
Webhook Ingress, Reconciler).

## 2. Границы

In scope: модель и запись события в транзакции, relay с `FOR UPDATE SKIP LOCKED`, метрики
интерфейсом, franz-go-адаптер публикации, reference-миграция SQL.
Out of scope: потребление событий (1.3), конкретные топики сервисов, Debezium CDC.

## 3. Функциональные требования

FR-1. Модель GORM `Record` (таблица `outbox`): `id int64`, `event_id uuid уникальный`,
`topic`, `key`, `headers jsonb`, `payload bytea`, `created_at`, `published_at` nullable.
Автосоздание таблицы запрещено: пакет поставляет файл `migrations/outbox.sql` (goose) для
копирования в сервис.

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
func (r *Relay) Run(ctx context.Context) error       // цикл с интервалом
func (r *Relay) PublishBatch(ctx context.Context) (int, error) // тестируемая единица
```

FR-3. `PublishBatch`: в транзакции выбрать пачку неопубликованных (`ORDER BY id LIMIT n
FOR UPDATE SKIP LOCKED`), опубликовать, проставить `published_at`; ошибка публикации -
rollback, событие останется для следующего цикла; пачки обрабатываются до опустошения.

FR-4. Адаптер `kafkapub`: `Publisher` поверх `franz-go` (`kgo.Client.ProduceSync`), продюсер
идемпотентный (`kgo.RequiredAcks(kgo.AllISRAcks())`, `kgo.ProducerBatchMaxBytes`, retries);
заголовки события переносятся в Kafka headers; ключ партиции - `Envelope.Key`.

FR-5. Метрики: интерфейс `Metrics` (лаг outbox, количество опубликованных), реализация noop по
умолчанию; Prometheus-адаптер подключается в 0.7.

## 4. Контракты

DDL строго из `CBL-003` (раздел 6). Заголовки: `event_id`, `event_type`, `occurred_at`,
`producer`, `traceparent`, `schema_version` (`envelope.proto`). Ошибки вводятся sentinel-ошибками,
панки запрещены.

## 5. Модель данных и миграции

`migrations/outbox.sql`: `CREATE TABLE outbox (...)` из `CBL-003` плюс индекс
`published_at` partial (`WHERE published_at IS NULL`) для эффективного polling.

## 6. Конечные автоматы

Не применимо.

## 7. Edge cases

Конкурентные relay-инстансы (`SKIP LOCKED`), падение после публикации до `published_at`
(at-least-once, дубль допустим, потребитель идемпотентен), пустая пачка, ошибки Kafka,
невалидный envelope, очень большой payload.

## 8. Нефункциональные требования

Лаг публикации при нормальной работе не более 1 секунды (интервал по умолчанию 250 мс,
батч 100). Relay не блокирует бизнес-транзакции. Unit-тесты без Docker; integration-тесты
(testcontainers PostgreSQL и Kafka) под тегом `integration`.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | валидация envelope, заголовки, нарезка тем, настройка опций |
| Integration | запись в транзакции + откат транзакции не оставляет outbox; параллельные relay; повторная публикация после сбоя |
| E2E | не применимо (проверяется в сервисах) |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.2: outbox с relay на SKIP LOCKED`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- CDC-вариант (Debezium) зафиксирован как альтернатива, не реализуется.
