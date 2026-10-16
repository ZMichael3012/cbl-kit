package money

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ParseJSONNumber parses a JSON number literal into Money.
// The exponent form (for example 1e-6 in provider payloads) is handled by exact
// decimal point shifting over the digit string, without float64.
func ParseJSONNumber(number json.Number, asset string, decimals uint32) (Money, error) {
	return parseDecimal(number.String(), asset, decimals, true)
}

// ParseAny parses a decoded JSON value into Money: strings and json.Number are
// accepted; float64 and float32 are rejected with ErrFloatForbidden, because the
// caller must decode numbers with json.Decoder.UseNumber; other types are invalid.
func ParseAny(value any, asset string, decimals uint32) (Money, error) {
	switch v := value.(type) {
	case string:
		return Parse(v, asset, decimals)
	case json.Number:
		return ParseJSONNumber(v, asset, decimals)
	case float64, float32:
		return Money{}, fmt.Errorf("%w: %T", ErrFloatForbidden, v)
	default:
		return Money{}, fmt.Errorf("%w: %T", ErrInvalidAmount, value)
	}
}

// parseDecimal parses a decimal literal with an optional exponent.
// The exponent is applied exactly: shift = decimals + exponent - fraction length;
// a positive shift appends zeros, a negative shift must be absorbed by trailing
// zeros of the fraction, otherwise the value needs more decimals than allowed.
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
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, literal)
	}
	var exponent int64
	if idx := strings.IndexAny(value, "eE"); idx >= 0 {
		if !allowExponent {
			return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, literal)
		}
		parsed, err := strconv.ParseInt(value[idx+1:], 10, 64)
		if err != nil {
			return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, literal)
		}
		exponent = parsed
		value = value[:idx]
	}
	intPart, fracPart, hasFrac := strings.Cut(value, ".")
	if intPart == "" || !isDigits(intPart) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, literal)
	}
	if hasFrac && (fracPart == "" || !isDigits(fracPart)) {
		return Money{}, fmt.Errorf("%w: value %q", ErrInvalidAmount, literal)
	}
	digits := intPart + fracPart
	shift := int64(decimals) + exponent - int64(len(fracPart))
	switch {
	case shift >= 0:
		// int64 has at most 19 digits; 25 keeps a safe margin and bounds the allocation.
		if int64(len(digits))+shift > 25 {
			return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, literal)
		}
		digits += strings.Repeat("0", int(shift))
	case shift < 0:
		for i := int64(0); i < -shift; i++ {
			if len(digits) == 0 {
				break
			}
			if digits[len(digits)-1] != '0' {
				return Money{}, fmt.Errorf("%w: value %q exceeds %d decimals", ErrInvalidAmount, literal, decimals)
			}
			digits = digits[:len(digits)-1]
		}
		if strings.Trim(digits, "0") == "" {
			return New(0, asset, decimals)
		}
	}
	units, err := parseMinorUnits(digits, neg)
	if err != nil {
		return Money{}, fmt.Errorf("%w: value %q", ErrOverflow, literal)
	}
	return New(units, asset, decimals)
}
