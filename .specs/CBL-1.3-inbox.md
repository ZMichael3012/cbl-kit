# CBL-1.3. Пакет inbox и идемпотентный consumer

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `inbox` |
| Размер | L |
| Волна и параллельная группа | W0-KIT |
| Зависит от | 1.2, 1.7 (CF-1) |
| Контракты | inbox DDL и правила retry/DLQ из `CBL-003`, раздел 6 |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Kafka дает at-least-once: каждый консьюмер обязан быть идемпотентным. Inbox-таблица
`(consumer, event_id)` в одной транзакции с эффектом обработки отсекает дубликаты; ошибки
уходят в retry-топики и DLQ (`CBL-003`, раздел 6).

## 2. Границы

In scope: Guard идемпотентности, Processor обработки сообщений, маршрутизация retry/DLQ.
Out of scope: конфигурация franz-go consumer-групп в сервисах, мониторинг лага (0.7).

## 3. Функциональные требования

FR-1. Модель GORM `Record` (таблица `inbox`): `consumer`, `event_id`, `processed_at`,
составной первичный ключ `(consumer, event_id)`. Reference-миграция `migrations/inbox.sql`.

FR-2. Guard:

```go
type Guard struct{}
func NewGuard() *Guard
// Process выполняет fn в одной транзакции с вставкой inbox-записи.
// Возвращает processed=false без вызова fn, если событие уже обработано.
func (g *Guard) Process(ctx context.Context, db *gorm.DB, consumer, eventID string, fn func(tx *gorm.DB) error) (processed bool, err error)
```

FR-3. Вставка inbox выполняется `INSERT ... ON CONFLICT DO NOTHING`; `RowsAffected == 0`
означает дубликат; ошибка `fn` откатывает всю транзакцию, включая inbox-запись.

FR-4. Processor:

```go
type Message struct{ Topic, Key string; Headers map[string]string; Value []byte }
type Handler interface{ Handle(ctx context.Context, db *gorm.DB, msg Message) error }
type Router struct{} // маршрутизация retry/DLQ
func NewRouter(mainTopic string, maxAttempts int) *Router
func (r *Router) Next(topic string, attempt int, err error) (string, bool) // topic, isDLQ
type Processor struct{}
func NewProcessor(db *gorm.DB, guard *Guard, handler Handler, pub outbox.Publisher, router *Router, log *zap.Logger) *Processor
func (p *Processor) Process(ctx context.Context, msg Message) error
```

FR-5. Правила повторных попыток: счетчик `x-retry-attempt` в заголовках; при ошибке обработки
сообщение публикуется в `<topic>.retry.5s` -> `.retry.1m` -> `.retry.10m` -> `<topic>.dlq`
(причина в заголовке `x-error`); `event_id` сохраняется, чтобы inbox-логика работала при
повторе. При исчерпании попыток - DLQ и подтверждение исходного сообщения.

FR-6. Тонкий адаптер `kafkaconsumer` (franz-go): `Run(ctx, topics, groupID, batchSize)` читает
сообщения и передает их в `Processor`; коммит офсета после успешной обработки или DLQ.

## 4. Контракты

DDL и правила из `CBL-003` (раздел 6). Заголовки: `event_id`, `event_type`, `traceparent`,
`x-retry-attempt`, `x-error`.

## 5. Модель данных и миграции

`migrations/inbox.sql` из `CBL-003`.

## 6. Конечные автоматы

Маршрутизация попыток: `main -> retry.5s -> retry.1m -> retry.10m -> dlq`.

## 7. Edge cases

Дубликат до и после обработки, ошибка обработчика после побочного эффекта (эффект и inbox в
одной транзакции), потеря `event_id`, сообщения не по порядку, пустые заголовки, конкурентная
обработка одного event_id разными инстансами (уникальный ключ).

## 8. Нефункциональные требования

Дубликат не вызывает повторного эффекта. Обработка одного сообщения - в одной транзакции БД.
Unit-тесты без Docker; integration (testcontainers) под тегом `integration`.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | Guard (вставка, дубликат, откат), Router (все переходы, DLQ), Processor (успех, ошибка, дубликат) |
| Integration | реальный PostgreSQL: конкурентные Process одного event_id; эффект + inbox атомарны |
| E2E | не применимо |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.3: inbox и идемпотентный consumer с retry/DLQ`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- Задержка retry реализуется через отдельные топики; точный таймер ожидания настраивается
  консьюмером сервиса, пакет задает только маршрут.
