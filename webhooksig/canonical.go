package webhooksig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// CanonicalJSON returns the deterministic compact JSON representation of raw:
// object keys are sorted, HTML escaping is off and number literals are preserved
// exactly through json.Number. Equal payloads canonize to equal bytes regardless
// of key order or formatting; CanonicalJSONHMAC signs this form, and provider
// profiles of the mock gateway (17.5) reuse it in contract tests.
func CanonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: body: %v", ErrMalformed, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: body has trailing data", ErrMalformed)
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrMalformed, err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
