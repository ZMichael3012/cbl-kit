package webhooksig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// CanonicalJSON returns the deterministic compact JSON representation of raw:
// object keys are sorted, HTML escaping is off, number literals are preserved
// exactly through json.Number and duplicate object keys or trailing data are
// rejected, so equal payloads canonize to equal bytes and the signed form is
// unambiguous. CanonicalJSONHMAC signs this form, and provider profiles of the
// mock gateway (17.5) reuse it in contract tests.
func CanonicalJSON(raw []byte) ([]byte, error) {
	value, err := decodeStrict(raw)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrMalformed, err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// decodeStrict decodes exactly one JSON value into any with number literals kept
// as json.Number; duplicate object keys and trailing data are malformed, because
// the canonical form and the token echo source must not be ambiguous.
func decodeStrict(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: body has trailing data", ErrMalformed)
	}
	return value, nil
}

// decodeValue reads one value from the token stream recursively.
func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: body: %v", ErrMalformed, err)
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("%w: body: %v", ErrMalformed, err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("%w: object key is not a string", ErrMalformed)
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("%w: duplicate object key %q", ErrMalformed, truncate(key))
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("%w: body: %v", ErrMalformed, err)
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("%w: body: %v", ErrMalformed, err)
		}
		return array, nil
	default:
		return nil, fmt.Errorf("%w: unexpected delimiter %q", ErrMalformed, delim)
	}
}

// truncate bounds attacker-controlled values quoted in error messages.
func truncate(value string) string {
	const maxDisplay = 64
	if len(value) <= maxDisplay {
		return value
	}
	return value[:maxDisplay] + "..."
}
