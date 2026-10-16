// Package money provides the CBL platform money type.
// Money is stored as int64 in asset minor units plus decimals,
// and floating point arithmetic is forbidden at the package API level.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaxDecimals sets the maximum allowed number of digits after the decimal point.
const MaxDecimals uint32 = 18

// Package errors are returned wrapped through fmt.Errorf with %w.
var (
	// ErrInvalidAsset reports an empty or invalid asset code.
	ErrInvalidAsset = errors.New("money: invalid asset")
	// ErrInvalidDecimals reports an invalid number of digits after the decimal point.
	ErrInvalidDecimals = errors.New("money: invalid decimals")
	// ErrInvalidAmount reports an invalid amount.
	ErrInvalidAmount = errors.New("money: invalid amount")
	// ErrFloatForbidden reports a float64 or float32 input; amounts must be
	// decoded with json.Decoder.UseNumber or passed as strings.
	ErrFloatForbidden = errors.New("money: float input is forbidden")
	// ErrAssetMismatch reports an asset code mismatch in an operation.
	ErrAssetMismatch = errors.New("money: asset mismatch")
	// ErrDecimalsMismatch reports a decimals mismatch in an operation.
	ErrDecimalsMismatch = errors.New("money: decimals mismatch")
	// ErrOverflow reports an int64 overflow in arithmetic.
	ErrOverflow = errors.New("money: overflow")
)

// Money describes a monetary amount in asset minor units.
type Money struct {
	// MinorUnits sets the amount in asset minor units.
	MinorUnits int64
	// Asset sets the asset code, for example USDT.
	Asset string
	// Decimals sets the number of digits after the decimal point.
	Decimals uint32
}

// New creates Money with asset and decimals validation.
func New(minorUnits int64, asset string, decimals uint32) (Money, error) {
	if strings.TrimSpace(asset) == "" {
		return Money{}, fmt.Errorf("%w: empty asset", ErrInvalidAsset)
	}
	if decimals > MaxDecimals {
		return Money{}, fmt.Errorf("%w: %d > %d", ErrInvalidDecimals, decimals, MaxDecimals)
	}
	return Money{MinorUnits: minorUnits, Asset: asset, Decimals: decimals}, nil
}

// Parse parses a decimal string into Money; exponential notation is forbidden.
func Parse(value, asset string, decimals uint32) (Money, error) {
	return parseDecimal(value, asset, decimals, false)
}

// MustParse parses the value and panics on error; it is allowed only for constants and tests.
func MustParse(value, asset string, decimals uint32) Money {
	m, err := Parse(value, asset, decimals)
	if err != nil {
		panic(err)
	}
	return m
}

// String renders a decimal string with exactly Decimals digits after the decimal point.
func (m Money) String() string {
	var u uint64
	if m.MinorUnits < 0 {
		// -(m.MinorUnits + 1) does not exceed MaxInt64, so the conversion is safe.
		u = uint64(-(m.MinorUnits + 1)) + 1 //nolint:gosec // G115: the value is always positive and within uint64 bounds
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

// Add adds amounts; it requires matching asset and decimals.
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

// Sub subtracts an amount; it requires matching asset and decimals.
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

// MulInt multiplies the amount by an integer with overflow checking.
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

// Neg returns the amount with the opposite sign; it returns an error for math.MinInt64.
func (m Money) Neg() (Money, error) {
	if m.MinorUnits == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: neg of min int64", ErrOverflow)
	}
	return Money{MinorUnits: -m.MinorUnits, Asset: m.Asset, Decimals: m.Decimals}, nil
}

// IsZero reports that the amount is zero.
func (m Money) IsZero() bool { return m.MinorUnits == 0 }

// Cmp compares amounts; it requires matching asset and decimals.
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

// checkCompatible validates matching asset and decimals before arithmetic.
func (m Money) checkCompatible(o Money) error {
	if m.Asset != o.Asset {
		return fmt.Errorf("%w: %q vs %q", ErrAssetMismatch, m.Asset, o.Asset)
	}
	if m.Decimals != o.Decimals {
		return fmt.Errorf("%w: %d vs %d", ErrDecimalsMismatch, m.Decimals, o.Decimals)
	}
	return nil
}

// isDigits reports that the string is non-empty and consists of digits only.
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

// parseMinorUnits converts a digit string to int64 taking the sign and int64 bounds into account.
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
	units := int64(u) //nolint:gosec // G115: u was checked against limit above (MaxInt64 or 1<<63)
	if neg {
		units = -units
	}
	return units, nil
}
