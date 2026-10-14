# CBL-1.5. HTTP middleware (Gin)

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `ginmw` |
| Размер | S |
| Волна и параллельная группа | W0-KIT |
| Зависит от | нет |
| Контракты | формат ошибок RFC 9457 из `CBL-003`, раздел 1 |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Единое поведение всех HTTP-сервисов на Gin (Billing API, Back-office, API Gateway,
Webhook Ingress): request_id, логирование доступа, ошибки в формате
`application/problem+json` (RFC 9457), защита от паник.

## 2. Границы

In scope: middleware и хелперы ошибок для Gin.
Out of scope: аутентификация, rate limit, метрики Prometheus (задачи E7, 0.7).

## 3. Функциональные требования

FR-1. `RequestID() gin.HandlerFunc`: берет `X-Request-Id` или генерирует UUIDv7, кладет в
контекст, возвращает в ответном заголовке.

FR-2. `AccessLog(log *zap.Logger) gin.HandlerFunc`: логирует метод, путь, статус, длительность,
request_id; ошибки уровня 5xx - error, 4xx - warn.

FR-3. Problem JSON:

```go
type Problem struct {
  Type, Title string; Status int; Code string; Detail string; RequestID string
}
func AbortWithProblem(c *gin.Context, status int, code, title, detail string)
func AbortFromError(c *gin.Context, err error) // маппинг grpcmw.Kind -> HTTP-статус
```

FR-4. `Recovery(log *zap.Logger) gin.HandlerFunc`: panic -> 500 problem+json, стек в лог,
ответ без внутренних деталей.

FR-5. Маппинг видов ошибок: NotFound -> 404, InvalidArgument -> 422, AlreadyExists -> 409,
FailedPrecondition -> 409, ResourceExhausted -> 429, Unavailable -> 503, Internal -> 500.

## 4. Контракты

Тело ошибки и `request_id` - `CBL-003`, раздел 1. Код `code` - машиночитаемая строка
(например, `idempotency_key_reuse`).

## 5. Модель данных и миграции

Не применимо.

## 6. Конечные автоматы

Не применимо.

## 7. Edge cases

Паника в хендлере, запись после `WriteHeader` (проверять `c.Writer.Written()`), пустой
`X-Request-Id`, длинные заголовки, вложенные ошибки.

## 8. Нефункциональные требования

Оверхед не более 30 мкс на запрос; логирование структурированное.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | httptest: request_id генерация и проброс, problem+json тело и заголовок, recovery, маппинг всех Kind, AccessLog |
| Integration | не применимо |
| E2E | не применимо |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.5: Gin middleware problem+json и request_id`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- Нет.
