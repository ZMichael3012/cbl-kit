package money

import (
	"fmt"
	"strconv"
	"strings"
)

// maxLiteralDigits bounds the formatted digit string: int64 has at most 19
// digits, 25 keeps a safe margin and bounds allocations for provider input.
const maxLiteralDigits = 25

// maxDisplayLiteral bounds literals quoted in error messages: provider payloads
// can carry arbitrarily long numeric strings and must not inflate logs.
const maxDisplayLiteral = 32

// parseDecimal parses a decimal literal with an optional exponent.
// The exponent is applied exactly: shift = decimals + exponent - fraction length;
// a positive shift appends zeros, a negative shift must be absorbed by trailing
// zeros of the digits, otherwise the value needs more decimals than allowed.
// Trailing fractional zeros beyond the scale and leading zeros are accepted
// (CBL-1.1, FR-4).
func parseDecimal(literal, asset string, decimals uint32, allowExponent bool) (Money, error) {
	if _, err := New(0, asset, decimals); err != nil {
		return Money{}, err
	}
	value := strings.TrimSpace(literal)
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
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, displayLiteral(literal))
	}
	var exponent int64
	if idx := strings.IndexAny(value, "eE"); idx >= 0 {
		if !allowExponent {
			return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, displayLiteral(literal))
		}
		parsed, err := strconv.ParseInt(value[idx+1:], 10, 64)
		if err != nil {
			return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, displayLiteral(literal))
		}
		exponent = parsed
		value = value[:idx]
	}
	intPart, fracPart, hasFrac := strings.Cut(value, ".")
	if intPart == "" || !isDigits(intPart) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, displayLiteral(literal))
	}
	if hasFrac && (fracPart == "" || !isDigits(fracPart)) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, displayLiteral(literal))
	}
	digits := intPart + fracPart
	if strings.Trim(digits, "0") == "" {
		return New(0, asset, decimals)
	}
	// Extreme exponents are classified before any arithmetic, so that the shift
	// computation below cannot overflow int64; the digits are nonzero here.
	if exponent > int64(maxLiteralDigits) {
		return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, displayLiteral(literal))
	}
	if exponent < -int64(maxLiteralDigits) {
		return Money{}, fmt.Errorf("%w: value %q exceeds %d decimals", ErrInvalidAmount, displayLiteral(literal), decimals)
	}
	// The scaled value is digits * 10^shift; all terms are small after the
	// exponent bounds above, so the arithmetic and |shift| stay within int64.
	shift := int64(decimals) + exponent - int64(len(fracPart))
	switch {
	case shift > int64(maxLiteralDigits):
		return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, displayLiteral(literal))
	case shift < -int64(maxLiteralDigits):
		return Money{}, fmt.Errorf("%w: value %q exceeds %d decimals", ErrInvalidAmount, displayLiteral(literal), decimals)
	case shift >= 0:
		digits = strings.TrimLeft(digits, "0")
		if int64(len(digits))+shift > maxLiteralDigits {
			return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, displayLiteral(literal))
		}
		digits += strings.Repeat("0", int(shift))
	default:
		// A negative shift must be absorbed by trailing zeros; for nonzero
		// digits the loop stops at the first nonzero digit from the right.
		for strip := -shift; strip > 0; strip-- {
			if len(digits) == 0 {
				break
			}
			if digits[len(digits)-1] != '0' {
				return Money{}, fmt.Errorf("%w: value %q exceeds %d decimals", ErrInvalidAmount, displayLiteral(literal), decimals)
			}
			digits = digits[:len(digits)-1]
		}
	}
	units, err := parseMinorUnits(digits, neg)
	if err != nil {
		return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, displayLiteral(literal))
	}
	return New(units, asset, decimals)
}

// displayLiteral truncates an attacker-controlled literal for error messages.
func displayLiteral(literal string) string {
	if len(literal) <= maxDisplayLiteral {
		return literal
	}
	return literal[:maxDisplayLiteral] + "..."
}
