# AGENTS.md - cbl-kit

Общая Go-библиотека платформы CBL: денежный тип, HMAC, middleware Gin и gRPC, outbox и inbox.
Обязательные глобальные правила: `../../AGENTS.md`, `../../.specs/CBL-001-development-process.md`,
`../../.specs/CBL-002-repos-and-docker.md` (раздел 6, библиотеки).
Длинное тире (U+2014) запрещено, используется только дефис.

## 1. Назначение и границы

In scope: переиспользуемые пакеты без доменной логики, reference-миграции outbox/inbox, тесты.

Out of scope: бизнес-логика сервисов, конкретные топики и схемы событий (в сервисах и
`cbl-protos`), Prometheus-адаптеры метрик (задача 0.7).

## 2. Пакеты

| Пакет | Задача | Потребители |
| :-- | :-- | :-- |
| `money` | CBL-1.1 | все сервисы |
| `hmac` | CBL-1.6 | `cbl-webhook-ingress`, `cbl-notification` |
| `ginmw` | CBL-1.5 | HTTP-сервисы |
| `grpcmw` | CBL-1.4 | gRPC-сервисы |
| `outbox` (+ `kafkapub`) | CBL-1.2 | Payment, Ledger, Payout, Webhook Ingress, Reconciler |
| `inbox` (+ `kafkaconsumer`) | CBL-1.3 | Payment, Ledger, Payout, Connector, Notification |

## 3. Архитектура и зависимости

```
cbl-kit/
├── money/          # int64 minor units + decimals, без float
├── hmac/           # подпись по сырому телу, окно replay
├── ginmw/          # request_id, problem+json, access log, recovery
├── grpcmw/         # ошибки, recovery, metadata, request_id
├── outbox/         # Store, Envelope, Relay, kafkapub
├── inbox/          # Guard, Processor, kafkaconsumer
└── migrations/     # reference SQL для сервисов
```

Зависимости: `gorm.io/gorm`, `gorm.io/driver/postgres`, `gin-gonic/gin`, `go.uber.org/zap`,
`google.golang.org/grpc`, `google.golang.org/protobuf`, `google/uuid`, `twmb/franz-go`,
`stretchr/testify`, `testcontainers-go` (только интеграционные тесты).

Правила: `AutoMigrate` запрещен; миграции в сервисах через goose, из этого репозитория
копируются reference-файлы; панки в бизнес-коде запрещены; ошибки через `%w`; float запрещен.

## 4. Команды

```powershell
go build ./...
go vet ./...
go test ./... -cover
go test -tags integration ./outbox/... ./inbox/...   # требует Docker (testcontainers)
docker run --rm -v "%CD%":/app -w /app golangci/golangci-lint:v1.64.8-alpine golangci-lint run ./...
powershell -File ../../scripts/check-no-emdash.ps1 -Root .
```

## 5. Правила разработки

- Конвейер: спека в `.specs/` -> тесты (unit; integration под тегом `integration`) -> код.
- Покрытие unit+integration не ниже 80% по репозиторию, для `money` не ниже 95%.
- Docstring на русском в стиле godoc для каждой экспортируемой сущности, предложение с точкой.
- Комментарии объясняют почему; имена экспортируемых сущностей без сокращений.
- Коммит: `CBL-<эпик>.<номер>: <краткое сообщение>`, спека в том же коммите.

## 6. Trade-offs

| ID | Решение | Плюсы | Минусы и компенсация |
| :-- | :-- | :-- | :-- |
| T-KK1 | `outbox.Publisher` интерфейс, franz-go только в адаптере `kafkapub` | тестируемость, сервисы не зависят от Kafka-клиента напрямую | плюс один слой; адаптер тонкий и покрыт unit-тестами |
| T-KK2 | reference-миграции SQL вместо встроенного AutoMigrate | сервис владеет схемой, история в goose | дублирование DDL в сервисах; файлы копируются как есть |
| T-KK3 | метрики интерфейсом, без Prometheus-зависимости | библиотека без тяжелых зависимостей | адаптер нужен сервисам; добавляется в 0.7 |
| T-KK4 | kafkaconsumer тонкий, логика в Processor | ядро покрыто unit-тестами без брокера | интеграция проверяется в сервисах |
| T-KK5 | Публикация Kafka внутри открытой транзакции relay | простота, минимальный лаг | сетевой вызов удерживает блокировки строк; метрика завышается при откате | батч не более 100, пересмотр на двухфазную схему по итогам 2.9 и 14.2 |
| T-KK6 | `storagex.Migrate` вынесен в kit | сервисы не дублируют обвязку goose | kit получает зависимость goose | зависимость только для рантайма миграций, в бизнес-коде не используется |
| T-KK7 | Обвязка main (health, metrics-заглушка, getenv) дублируется в сервисах | сервис автономен и прост | дублирование при 13+ репозиториях | вынос в cbl-kit/httpx после 0.7, когда появятся реальные метрики |

## 7. Known issues

| ID | Проблема | Влияние | План | Статус |
| :-- | :-- | :-- | :-- | :-- |
| KI-K1 | Интеграционные тесты требуют Docker | без Docker нет проверки конкурентности relay | тег `integration`, CI в 0.2 | open |
| KI-K2 | Kafka-интеграция outbox не покрыта по умолчанию | адаптер проверяется unit-тестами | Kafka-контейнер в CI задачей 0.2 | open |
| KI-K3 | Метрика AddPublished считается до commit транзакции relay | возможное завышение при откате публикации | перенести учет после commit в рамках 2.9 | open |

## 8. Журнал задач

| CBL | Дата | Статус | Спека | Коммит |
| :-- | :-- | :-- | :-- | :-- |
| CBL-1.1 - 1.6 | 2026-10-14 | done, ревью W1 закрыто (unit+integration: outbox 82.8%, inbox 85.0%, money 96.3%; DLQ для poison-сообщений, storagex, таймаут доставки Kafka) | `.specs/CBL-1.*.md` | ожидает коммита |
| CBL-1.3 (fix) | 2026-10-14 | done, находки ревью 2.7: DLQ-фолбэки для poison-сообщений (невалидный event_id санитизируется в `x-original-event-id`; отсутствующий event_type -> `unknown`; битый конверт повтора уходит в DLQ, а не зацикливает партицию) | `inbox/processor.go`, тесты inbox | этот коммит |

## 9. Runbook

Диагностика отставания outbox:

1. Метрика pending (Prometheus после 0.7) и логи relay: `published batch`, `publish failed`.
2. Проверить доступность Kafka (`KAFKA_BROKERS`), права продюсера, размер батча.
3. При ошибке публикации события остаются в таблице и переотправляются следующим циклом;
   потеря невозможна, возможны дубли (at-least-once), потребители идемпотентны через inbox.
