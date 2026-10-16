package webhooksig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/webhooksig"
)

func TestCanonicalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "sorts object keys", input: `{"b":1,"a":2}`, want: `{"a":2,"b":1}`},
		{name: "preserves number literals", input: `{"n":1.50,"e":1e+2}`, want: `{"e":1e+2,"n":1.50}`},
		{name: "no html escaping", input: `{"s":"a<b&c>"}`, want: `{"s":"a<b&c>"}`},
		{name: "unescapes html entities", input: `{"s":"\u003cb\u003e"}`, want: `{"s":"<b>"}`},
		{name: "trims whitespace", input: ` { "a" : [ 1 , 2 ] } `, want: `{"a":[1,2]}`},
		{name: "nested structures", input: `[{"z":{"y":null},"a":[true,false]}]`, want: `[{"a":[true,false],"z":{"y":null}}]`},
		{name: "keeps unicode", input: `{"s":"caf\u00e9"}`, want: `{"s":"café"}`},
		{name: "empty object", input: `{}`, want: `{}`},
		{name: "empty array", input: `[]`, want: `[]`},
		{name: "malformed", input: `{`, wantErr: true},
		{name: "trailing data", input: `{"a":1} x`, wantErr: true},
		{name: "duplicate keys", input: `{"a":1,"a":2}`, wantErr: true},
		{name: "nested duplicate keys", input: `{"b":{"c":1,"c":2}}`, wantErr: true},
		{name: "empty body", input: ``, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := webhooksig.CanonicalJSON([]byte(tt.input))
			if tt.wantErr {
				require.ErrorIs(t, err, webhooksig.ErrMalformed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestCanonicalJSONIsStable(t *testing.T) {
	t.Parallel()
	first, err := webhooksig.CanonicalJSON([]byte(`{"b":{"d":2,"c":1},"a":1}`))
	require.NoError(t, err)
	second, err := webhooksig.CanonicalJSON([]byte("{\n  \"a\": 1,\n  \"b\": {\"c\": 1, \"d\": 2}\n}"))
	require.NoError(t, err)
	require.Equal(t, string(first), string(second))
}
