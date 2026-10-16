package money_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/money"
)

func TestParseJSONNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    string
		asset    string
		decimals uint32
		want     int64
		wantErr  error
	}{
		{name: "plain decimal", value: "100.5", asset: "USDT", decimals: 6, want: 100500000},
		{name: "fewer decimals than scale", value: "100.5", asset: "USDT", decimals: 1, want: 1005},
		{name: "integer", value: "42", asset: "USDT", decimals: 0, want: 42},
		{name: "negative exponent", value: "1e-6", asset: "BTC", decimals: 8, want: 100},
		{name: "negative exponent one digit less", value: "1e-7", asset: "BTC", decimals: 8, want: 10},
		{name: "negative exponent beyond scale", value: "1e-9", asset: "BTC", decimals: 8, wantErr: money.ErrInvalidAmount},
		{name: "positive exponent shifts point", value: "12.34e2", asset: "USDT", decimals: 2, want: 123400},
		{name: "upper exponent with plus", value: "1E+2", asset: "USDT", decimals: 0, want: 100},
		{name: "fraction with exponent", value: "0.1e1", asset: "USDT", decimals: 0, want: 1},
		{name: "negative with exponent", value: "-1.5e1", asset: "USDT", decimals: 1, want: -150},
		{name: "zero with large negative exponent", value: "0e-30", asset: "USDT", decimals: 2, want: 0},
		{name: "zero with max exponent", value: "0e9223372036854775807", asset: "USDT", decimals: 2, want: 0},
		{name: "max exponent overflow", value: "1e9223372036854775807", asset: "USDT", decimals: 2, wantErr: money.ErrOverflow},
		{name: "min exponent needs decimals", value: "1e-9223372036854775808", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "trailing fractional zeros beyond scale", value: "1.230", asset: "USDT", decimals: 2, want: 123},
		{name: "trailing zero beyond scale", value: "1.50", asset: "USDT", decimals: 1, want: 15},
		{name: "long leading zeros", value: "00000000000000000000000000000001", asset: "USDT", decimals: 0, want: 1},
		{name: "positive shift overflow", value: "1e25", asset: "USDT", decimals: 8, wantErr: money.ErrOverflow},
		{name: "long digits with shift overflow", value: "123456789012345678901e5", asset: "USDT", decimals: 0, wantErr: money.ErrOverflow},
		{name: "long fraction needs decimals", value: "0.123456789012345678901234567890", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "trailing zeros absorbed by exponent", value: "10e-2", asset: "USDT", decimals: 1, want: 1},
		{name: "max int64 boundary", value: "92233720368547758.07", asset: "USDT", decimals: 2, want: 9223372036854775807},
		{name: "overflow", value: "92233720368547758.08", asset: "USDT", decimals: 2, wantErr: money.ErrOverflow},
		{name: "huge exponent overflow", value: "1e100", asset: "USDT", decimals: 6, wantErr: money.ErrOverflow},
		{name: "too many decimals", value: "0.1234567", asset: "USDT", decimals: 6, wantErr: money.ErrInvalidAmount},
		{name: "exponent needs more decimals", value: "1.5e-2", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "empty", value: "", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "not a number", value: "abc", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "broken exponent", value: "1e", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "exponent sign only", value: "1e+", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "two points", value: "1.2.3", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "double sign", value: "--1", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "empty asset", value: "1", asset: "", decimals: 2, wantErr: money.ErrInvalidAsset},
		{name: "too many decimals scale", value: "1", asset: "USDT", decimals: 19, wantErr: money.ErrInvalidDecimals},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := money.ParseJSONNumber(json.Number(tt.value), tt.asset, tt.decimals)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.MinorUnits)
			require.Equal(t, tt.asset, got.Asset)
		})
	}
}

func TestParseAny(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   any
		want    int64
		wantErr error
	}{
		{name: "string", value: "10.5", want: 1050000000},
		{name: "json number", value: json.Number("10.5"), want: 1050000000},
		{name: "json number exponent", value: json.Number("1e-6"), want: 100},
		{name: "float64 forbidden", value: 10.5, wantErr: money.ErrFloatForbidden},
		{name: "float32 forbidden", value: float32(10.5), wantErr: money.ErrFloatForbidden},
		{name: "int not a boundary type", value: 10, wantErr: money.ErrInvalidAmount},
		{name: "nil", value: nil, wantErr: money.ErrInvalidAmount},
		{name: "bool", value: true, wantErr: money.ErrInvalidAmount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := money.ParseAny(tt.value, "BTC", 8)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.MinorUnits)
		})
	}
}

func TestParseAnyRejectsFloatFromDecoderWithoutUseNumber(t *testing.T) {
	t.Parallel()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"price": 10.5}`), &payload))
	_, err := money.ParseAny(payload["price"], "USDT", 6)
	require.True(t, errors.Is(err, money.ErrFloatForbidden))
}
