// Пакет money предоставляет денежный тип платформы CBL.
// Деньги хранятся как int64 в минимальных единицах актива плюс decimals,
// арифметика с плавающей точкой запрещена на уровне API пакета.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaxDecimals задает максимальное допустимое количество знаков после точки.
const MaxDecimals uint32 = 18

// Ошибки пакета возвращаются обернутыми через fmt.Errorf с %w.
var (
	// ErrInvalidAsset сообщает о пустом или некорректном коде актива.
	ErrInvalidAsset = errors.New("money: invalid asset")
	// ErrInvalidDecimals сообщает о недопустимом количестве знаков после точки.
	ErrInvalidDecimals = errors.New("money: invalid decimals")
	// ErrInvalidAmount сообщает о некорректной сумме.
	ErrInvalidAmount = errors.New("money: invalid amount")
	// ErrAssetMismatch сообщает о несовпадении кодов активов при операции.
	ErrAssetMismatch = errors.New("money: asset mismatch")
	// ErrDecimalsMismatch сообщает о несовпадении decimals при операции.
	ErrDecimalsMismatch = errors.New("money: decimals mismatch")
	// ErrOverflow сообщает о переполнении int64 при арифметике.
	ErrOverflow = errors.New("money: overflow")
)

// Money описывает денежную сумму в минимальных единицах актива.
type Money struct {
	// MinorUnits задает сумму в минимальных единицах актива.
	MinorUnits int64
	// Asset задает код актива, например USDT.
	Asset string
	// Decimals задает количество знаков после точки.
	Decimals uint32
}

// New создает Money с валидацией актива и decimals.
func New(minorUnits int64, asset string, decimals uint32) (Money, error) {
	if strings.TrimSpace(asset) == "" {
		return Money{}, fmt.Errorf("%w: empty asset", ErrInvalidAsset)
	}
	if decimals > MaxDecimals {
		return Money{}, fmt.Errorf("%w: %d > %d", ErrInvalidDecimals, decimals, MaxDecimals)
	}
	return Money{MinorUnits: minorUnits, Asset: asset, Decimals: decimals}, nil
}

// Parse разбирает десятичную строку в Money; экспоненциальная запись запрещена.
func Parse(value, asset string, decimals uint32) (Money, error) {
	if _, err := New(0, asset, decimals); err != nil {
		return Money{}, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return Money{}, fmt.Errorf("%w: empty value", ErrInvalidAmount)
	}
	neg := false
	switch value[0] {
	case '+':
		value = value[1:]
	case '-':
		neg = true
		value = value[1:]
	}
	if value == "" {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, value)
	}
	intPart, fracPart, hasFrac := strings.Cut(value, ".")
	if intPart == "" || !isDigits(intPart) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, value)
	}
	if hasFrac && (fracPart == "" || !isDigits(fracPart)) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, value)
	}
	if len(fracPart) > int(decimals) {
		return Money{}, fmt.Errorf("%w: %q exceeds %d decimals", ErrInvalidAmount, value, decimals)
	}
	fracPart += strings.Repeat("0", int(decimals)-len(fracPart))
	digits := intPart + fracPart
	units, err := parseMinorUnits(digits, neg)
	if err != nil {
		return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, value)
	}
	return New(units, asset, decimals)
}

// MustParse разбирает строку и паникует при ошибке; допустим только для констант и тестов.
func MustParse(value, asset string, decimals uint32) Money {
	m, err := Parse(value, asset, decimals)
	if err != nil {
		panic(err)
	}
	return m
}

// String печатает десятичную строку с ровно Decimals знаками после точки.
func (m Money) String() string {
	var u uint64
	if m.MinorUnits < 0 {
		// -(m.MinorUnits + 1) не превышает MaxInt64, преобразование безопасно.
		u = uint64(-(m.MinorUnits + 1)) + 1 //nolint:gosec // G115: значение всегда положительно и в границах uint64
	} else {
		u = uint64(m.MinorUnits)
	}
	digits := strconv.FormatUint(u, 10)
	neg := m.MinorUnits < 0

	if m.Decimals == 0 {
		if neg {
			return "-" + digits
		}
		return digits
	}
	if len(digits) <= int(m.Decimals) {
		digits = strings.Repeat("0", int(m.Decimals)-len(digits)+1) + digits
	}
	split := len(digits) - int(m.Decimals)
	out := digits[:split] + "." + digits[split:]
	if neg {
		out = "-" + out
	}
	return out
}

// Add складывает суммы; требует совпадения актива и decimals.
func (m Money) Add(o Money) (Money, error) {
	if err := m.checkCompatible(o); err != nil {
		return Money{}, err
	}
	sum := m.MinorUnits + o.MinorUnits
	if (o.MinorUnits > 0 && sum < m.MinorUnits) || (o.MinorUnits < 0 && sum > m.MinorUnits) {
		return Money{}, fmt.Errorf("%w: add", ErrOverflow)
	}
	return Money{MinorUnits: sum, Asset: m.Asset, Decimals: m.Decimals}, nil
}

// Sub вычитает сумму; требует совпадения актива и decimals.
func (m Money) Sub(o Money) (Money, error) {
	if err := m.checkCompatible(o); err != nil {
		return Money{}, err
	}
	diff := m.MinorUnits - o.MinorUnits
	if (o.MinorUnits < 0 && diff < m.MinorUnits) || (o.MinorUnits > 0 && diff > m.MinorUnits) {
		return Money{}, fmt.Errorf("%w: sub", ErrOverflow)
	}
	return Money{MinorUnits: diff, Asset: m.Asset, Decimals: m.Decimals}, nil
}

// MulInt умножает сумму на целое число с проверкой переполнения.
func (m Money) MulInt(k int64) (Money, error) {
	if m.MinorUnits == 0 || k == 0 {
		return Money{MinorUnits: 0, Asset: m.Asset, Decimals: m.Decimals}, nil
	}
	if m.MinorUnits == math.MinInt64 && k == -1 {
		return Money{}, fmt.Errorf("%w: mul", ErrOverflow)
	}
	if k == 1 {
		return m, nil
	}
	res := m.MinorUnits * k
	if res/k != m.MinorUnits {
		return Money{}, fmt.Errorf("%w: mul", ErrOverflow)
	}
	return Money{MinorUnits: res, Asset: m.Asset, Decimals: m.Decimals}, nil
}

// Neg возвращает сумму с противоположным знаком; для math.MinInt64 возвращает ошибку.
func (m Money) Neg() (Money, error) {
	if m.MinorUnits == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: neg of min int64", ErrOverflow)
	}
	return Money{MinorUnits: -m.MinorUnits, Asset: m.Asset, Decimals: m.Decimals}, nil
}

// IsZero сообщает, что сумма равна нулю.
func (m Money) IsZero() bool { return m.MinorUnits == 0 }

// Cmp сравнивает суммы; требует совпадения актива и decimals.
func (m Money) Cmp(o Money) (int, error) {
	if err := m.checkCompatible(o); err != nil {
		return 0, err
	}
	switch {
	case m.MinorUnits < o.MinorUnits:
		return -1, nil
	case m.MinorUnits > o.MinorUnits:
		return 1, nil
	default:
		return 0, nil
	}
}

// checkCompatible проверяет совпадение актива и decimals перед арифметикой.
func (m Money) checkCompatible(o Money) error {
	if m.Asset != o.Asset {
		return fmt.Errorf("%w: %q vs %q", ErrAssetMismatch, m.Asset, o.Asset)
	}
	if m.Decimals != o.Decimals {
		return fmt.Errorf("%w: %d vs %d", ErrDecimalsMismatch, m.Decimals, o.Decimals)
	}
	return nil
}

// isDigits сообщает, что строка непуста и состоит только из цифр.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseMinorUnits переводит строку цифр в int64 с учетом знака и границ int64.
func parseMinorUnits(digits string, neg bool) (int64, error) {
	u, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, err
	}
	limit := uint64(math.MaxInt64)
	if neg {
		limit = 1 << 63
	}
	if u > limit {
		return 0, ErrOverflow
	}
	if neg && u == 1<<63 {
		return math.MinInt64, nil
	}
	units := int64(u) //nolint:gosec // G115: u проверен сверху limit (MaxInt64 или 1<<63)
	if neg {
		units = -units
	}
	return units, nil
}
