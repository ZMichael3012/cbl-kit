# CBL-1.1. Money type

| Field | Value |
| :-- | :-- |
| Epic | E1, shared library |
| Repository | `cbl-kit`, package `money` |
| Size | S |
| Wave and parallel group | W0-KIT |
| Depends on | none |
| Contracts | unchanged |
| Status | done (locally, awaiting commit) |

## 1. Context and goal

Money type for the entire platform: `int64` in minor units plus the asset `decimals`
(`CBL-000`, invariant 1). Float is forbidden entirely, including intermediate calculations.

## 2. Boundaries

In scope: the `money` package in `cbl-kit`.
Out of scope: rates and quotes (Quote), decimal arithmetic of rates.

## 3. Functional requirements

FR-1. The `Money` type (a value): fields `MinorUnits int64`, `Asset string`, `Decimals uint32`.

FR-2. Package API:

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

FR-3. `Add`/`Sub`/`Cmp` require matching `Asset` and `Decimals`, otherwise an error; int64
overflow is detected (including `Sub` at the boundaries and `Neg` of `math.MinInt64`
returning an error through a separate API variant if `Neg` cannot be error-free - to choose
and justify in the spec).

FR-4. `Parse` accepts a decimal string without an exponent, with at most `decimals` digits
after the decimal point; extra zeros are allowed; negative values are allowed; an empty
string, a point without digits, whitespace - an error.

FR-5. `String` prints a decimal string with exactly `decimals` digits after the decimal point.

## 4. Contracts

Compatible with the REST rule from `CBL-003` (a decimal string plus the asset code) and
`billing.common.v1.Money` (minor_units, asset, decimals).

## 5. Data model and migrations

Not applicable.

## 6. State machines

Not applicable.

## 7. Edge cases

`math.MaxInt64`/`math.MinInt64`, overflow in Add/Sub/MulInt, decimals = 0 and 18,
`"100"`, `"100.50"`, `"0.000001"`, `"-1.23"`, `"1e2"` (error), leading zeros, long strings.

## 8. Non-functional requirements

The package has no external dependencies other than the stdlib. Package test coverage is at
least 95%.

## 9. Test plan

| Level | Scenarios |
| :-- | :-- |
| Smoke | `go build ./...`, `go vet ./...` |
| Unit | table-driven tests of all functions, int64 boundaries, overflows, format/parse round-trip |
| Integration | not applicable |
| E2E | not applicable |
| Load | not applicable |

## 10. Work plan and commit

Commit `CBL-1.1: Money type with arithmetic without float`; no commits in the current session.

## 11. Open questions

- Whether `Split`/`Allocate` are needed for distributing amounts: leave to task 2.4 if required.
