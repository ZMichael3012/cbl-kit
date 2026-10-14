# CBL-1.1. Тип Money

| Поле | Значение |
| :-- | :-- |
| Эпик | E1, общая библиотека |
| Репозиторий | `cbl-kit`, пакет `money` |
| Размер | S |
| Волна и параллельная группа | W0-KIT |
| Зависит от | нет |
| Контракты | не меняются |
| Статус | done (локально, ожидает коммита) |

## 1. Контекст и цель

Денежный тип для всей платформы: `int64` в минимальных единицах плюс `decimals` актива
(`CBL-000`, инвариант 1). Float запрещен полностью, включая промежуточные вычисления.

## 2. Границы

In scope: пакет `money` в `cbl-kit`.
Out of scope: курсы и котировки (Quote), Decimal-арифметика курсов.

## 3. Функциональные требования

FR-1. Тип `Money` (значение): поля `MinorUnits int64`, `Asset string`, `Decimals uint32`.

FR-2. API пакета:

```go
func New(minorUnits int64, asset string, decimals uint32) (Money, error)
func Parse(value, asset string, decimals uint32) (Money, error)
func MustParse(value, asset string, decimals uint32) Money
func (m Money) String() string
func (m Money) Add(o Money) (Money, error)
func (m Money) Sub(o Money) (Money, error)
func (m Money) MulInt(k int64) (Money, error)
func (m Money) Neg() Money
func (m Money) IsZero() bool
func (m Money) Cmp(o Money) (int, error)
```

FR-3. `Add`/`Sub`/`Cmp` требуют совпадения `Asset` и `Decimals`, иначе ошибка; переполнение
int64 детектируется (в том числе `Sub` у границ и `Neg` от `math.MinInt64` возвращает ошибку
через отдельный вариант API, если Neg не может быть безошибочным - выбрать и обосновать в спеке).

FR-4. `Parse` принимает десятичную строку без экспоненты, не более `decimals` знаков после
точки; лишние нули допустимы; отрицательные значения допустимы; пустая строка, точка без цифр,
пробелы - ошибка.

FR-5. `String` печатает десятичную строку с ровно `decimals` знаками после точки.

## 4. Контракты

Совместим с REST-правилом `CBL-003` (строка-десятичное число плюс код актива) и
`billing.common.v1.Money` (minor_units, asset, decimals).

## 5. Модель данных и миграции

Не применимо.

## 6. Конечные автоматы

Не применимо.

## 7. Edge cases

`math.MaxInt64`/`math.MinInt64`, переполнение при Add/Sub/MulInt, decimals = 0 и 18,
`"100"`, `"100.50"`, `"0.000001"`, `"-1.23"`, `"1e2"` (ошибка), ведущие нули, длинные строки.

## 8. Нефункциональные требования

Пакет без внешних зависимостей кроме stdlib. Покрытие тестами пакета не ниже 95%.

## 9. План тестов

| Уровень | Сценарии |
| :-- | :-- |
| Smoke | `go build ./...`, `go vet ./...` |
| Unit | табличные тесты всех функций, границы int64, переполнения, формат/парсинг round-trip |
| Integration | не применимо |
| E2E | не применимо |
| Load | не применимо |

## 10. План работ и коммит

Коммит `CBL-1.1: тип Money с арифметикой без float`; в текущей сессии без коммитов.

## 11. Открытые вопросы

- Нужен ли `Split`/`Allocate` для распределения сумм: оставить на задачу 2.4 при необходимости.
