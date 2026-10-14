# CBL-1.6. HMAC-подпись и защита от replay

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `hmac` |
| Размер | S |
| Волна и параллельная группа | W0-KIT |
| Зависит от | нет |
| Контракты | формат подписи из `CBL-003`, раздел 4 |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Подпись вебхуков в обе стороны: входящие от шлюза (Webhook Ingress) и исходящие мерчантам
(Notification). Подпись считается по сырому телу, окно replay ограничено.

## 2. Границы

In scope: вычисление и проверка подписи, разбор заголовка, окно replay, ротация секретов.
Out of scope: регистрация webhook-endpoints и хранение секретов (задача 8.1), защита URL (SSRF, 8.2).

## 3. Функциональные требования

FR-1. Формат заголовка: `t=<unix>,v1=<hex>[,v1=<hex>]` (несколько v1 для ротации).

FR-2. API:

```go
const DefaultWindow = 5 * time.Minute
func Sign(secret []byte, timestamp time.Time, body []byte) string // hex HMAC-SHA256(secret, t + "." + body)
func BuildHeader(secrets ...Signature) string
func Verify(secret []byte, header string, body []byte, now time.Time, window time.Duration) error
func VerifyAny(secrets [][]byte, header string, body []byte, now time.Time, window time.Duration) error
func ParseHeader(header string) (timestamp time.Time, signatures [][]byte, err error)
```

FR-3. Подпись считается по `strconv.FormatInt(t, 10) + "." + body`; тело не должно
перекодироваться. Проверка: совпадение хотя бы одной подписи из `v1` (constant-time сравнение
`hmac.Equal`), возраст `|now - t| <= window`, иначе ошибка.

FR-4. Ошибки: `ErrMalformed`, `ErrSignatureMismatch`, `ErrExpired` (sentinel); оборачиваются
через `fmt.Errorf` с `%w`.

## 4. Контракты

Совместим с `X-Billing-Signature` из `CBL-003`, раздел 4, и проверкой вебхуков провайдера
(провайдерские заголовки маппятся сервисом в этот формат).

## 5. Модель данных и миграции

Не применимо.

## 6. Конечные автоматы

Не применимо.

## 7. Edge cases

Пустое тело, отсутствие `t` или `v1`, несколько подписей, timestamp в будущем, ровно на границе
окна, не-hex подпись, пустой секрет (ошибка конфигурации), очень длинное тело (без лимита длины,
лимит на уровне HTTP-сервера).

## 8. Нефункциональные требования

Constant-time сравнение; вычисление HMAC-SHA256 на телах до 1 МБ не более 5 мс.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...` |
| Unit | Sign/Verify round-trip, изменение тела, просроченный/будущий ts, ротация (2 подписи), malformed, граница окна |
| Integration | не применимо |
| E2E | не применимо |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.6: HMAC-подпись с защитой от replay`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- Нет.
