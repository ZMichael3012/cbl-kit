# CBL-1.4. gRPC middleware

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `grpcmw` |
| Размер | M |
| Волна и параллельная группа | W0-KIT |
| Зависит от | 1.7 (CF-1) |
| Контракты | маппинг ошибок и дедлайны из `CBL-003`, раздел 5 |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Единые интерсепторы для всех gRPC-сервисов: идентификатор запроса, структурированное
логирование, защита от паник, маппинг доменных ошибок в gRPC-коды, проброс `request_id` и
`traceparent`, извлечение `x-merchant-id` после mTLS.

## 2. Границы

In scope: серверные и клиентские unary/stream интерсепторы, определение типов доменных ошибок.
Out of scope: конкретные доменные ошибки сервисов, настройка TLS-креденшелов (в сервисах).

## 3. Функциональные требования

FR-1. Пакет определяет `errorsx`-совместимые виды ошибок (внутри `grpcmw`):

```go
type Kind uint8 // NotFound, InvalidArgument, AlreadyExists, FailedPrecondition, ResourceExhausted, Unavailable, Internal
type Error struct{ Kind Kind; Code string; Message string; Err error }
func E(kind Kind, code, message string, err error) *Error
func KindOf(err error) Kind
```

FR-2. `UnaryServer(log *zap.Logger) grpc.UnaryServerInterceptor` объединяет: recovery
(panic -> Internal, логирование стека, panic не выходит наружу), request_id (из metadata или
новый UUIDv7, кладется в ответные заголовки), логирование (метод, код, длительность),
маппинг ошибок.

FR-3. `MapError(err error) error`: `Error` -> `status.Error` с кодом
(`NotFound->codes.NotFound`, `InvalidArgument->InvalidArgument`, `AlreadyExists->AlreadyExists`,
`FailedPrecondition->FailedPrecondition`, `ResourceExhausted->ResourceExhausted`,
`Unavailable->Unavailable`, `Internal->Internal`); неизвестная ошибка -> `Internal` с
сообщением "internal error" без утечки деталей.

FR-4. `UnaryClient(log *zap.Logger) grpc.UnaryClientInterceptor`: проброс `request_id` и
`traceparent` в metadata, логирование исходящих вызовов и ошибок; лишние дедлайны не ставит.

FR-5. `MerchantID(ctx) (string, error)`: читает `x-merchant-id` из входящей metadata; пустое
значение - `InvalidArgument`. Helper `RequestID(ctx) string`.

FR-6. Stream-версии интерсепторов `StreamServer`, `StreamClient` с теми же правилами.

FR-7. Трассировка: пакет не дублирует `otelgrpc`; в README указано подключать
`otelgrpc.NewServerHandler()` отдельно.

## 4. Контракты

Маппинг кодов и правило бюджета дедлайнов - `CBL-003`, раздел 5. Metadata: `x-merchant-id`,
`request_id`, `traceparent`.

## 5. Модель данных и миграции

Не применимо.

## 6. Конечные автоматы

Не применимо.

## 7. Edge cases

Паника обработчика, ошибка без Kind (Internal), вложенные ошибки через `%w`, отсутствующая
metadata, неверный формат request_id (перегенерация), конкурентные вызовы.

## 8. Нефункциональные требования

Оверхед интерсепторов не более 50 мкс на вызов. Unit-тесты через `bufconn` без сети.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | bufconn-сервер: успех, каждый Kind -> код, паника, request_id проброс, MerchantID |
| Integration | не применимо |
| E2E | не применимо |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.4: gRPC middleware с маппингом ошибок`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- Бюджет дедлайна на клиенте оставлен сервисам: значения берутся из `CBL-003`, раздел 5.
