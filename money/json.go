package money

import (
	"encoding/json"
	"fmt"
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
