package money_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/money"
)

func TestNew(t *testing.T) {
	t.Parallel()

	m, err := money.New(10050, "USDT", 2)
	require.NoError(t, err)
	assert.Equal(t, int64(10050), m.MinorUnits)
	assert.Equal(t, "USDT", m.Asset)
	assert.Equal(t, uint32(2), m.Decimals)

	_, err = money.New(0, "", 2)
	require.ErrorIs(t, err, money.ErrInvalidAsset)

	_, err = money.New(0, "   ", 2)
	require.ErrorIs(t, err, money.ErrInvalidAsset)

	_, err = money.New(0, "USDT", 19)
	require.ErrorIs(t, err, money.ErrInvalidDecimals)
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		asset    string
		decimals uint32
		want     int64
		wantErr  error
	}{
		{name: "integer", value: "100", asset: "USDT", decimals: 2, want: 10000},
		{name: "decimal", value: "100.50", asset: "USDT", decimals: 2, want: 10050},
		{name: "smallest unit", value: "0.000001", asset: "BTC", decimals: 6, want: 1},
		{name: "negative", value: "-1.23", asset: "USDT", decimals: 2, want: -123},
		{name: "plus sign", value: "+1.23", asset: "USDT", decimals: 2, want: 123},
		{name: "zero decimals", value: "42", asset: "SATS", decimals: 0, want: 42},
		{name: "max int64", value: "92233720368547758.07", asset: "USDT", decimals: 2, want: math.MaxInt64},
		{name: "min int64", value: "-92233720368547758.08", asset: "USDT", decimals: 2, want: math.MinInt64},
		{name: "empty", value: "", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "sign only", value: "-", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "leading dot", value: ".5", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "trailing dot", value: "5.", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "exponent", value: "1e2", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "spaces", value: "1 000", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "too many decimals", value: "12.345", asset: "USDT", decimals: 2, wantErr: money.ErrInvalidAmount},
		{name: "overflow", value: "92233720368547758.08", asset: "USDT", decimals: 2, wantErr: money.ErrOverflow},
		{name: "negative overflow", value: "-92233720368547758.09", asset: "USDT", decimals: 2, wantErr: money.ErrOverflow},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := money.Parse(tt.value, tt.asset, tt.decimals)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, m.MinorUnits)
		})
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		decimals uint32
		want     string
	}{
		{value: "100.50", decimals: 2, want: "100.50"},
		{value: "100", decimals: 2, want: "100.00"},
		{value: "0.000001", decimals: 6, want: "0.000001"},
		{value: "-1.23", decimals: 2, want: "-1.23"},
		{value: "42", decimals: 0, want: "42"},
		{value: "-0.01", decimals: 2, want: "-0.01"},
		{value: "0", decimals: 4, want: "0.0000"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			m := money.MustParse(tt.value, "USDT", tt.decimals)
			assert.Equal(t, tt.want, m.String())
			back := money.MustParse(m.String(), "USDT", tt.decimals)
			assert.Equal(t, m, back)
		})
	}

	assert.Equal(t, "-92233720368547758.08", money.MustParse("-92233720368547758.08", "USDT", 2).String())
	assert.Equal(t, "92233720368547758.07", money.MustParse("92233720368547758.07", "USDT", 2).String())
}

func TestAddSub(t *testing.T) {
	t.Parallel()

	a := money.MustParse("100.50", "USDT", 2)
	b := money.MustParse("0.50", "USDT", 2)
	other := money.MustParse("1", "BTC", 2)
	otherDecimals := money.MustParse("1", "USDT", 6)

	sum, err := a.Add(b)
	require.NoError(t, err)
	assert.Equal(t, "101.00", sum.String())

	diff, err := a.Sub(b)
	require.NoError(t, err)
	assert.Equal(t, "100.00", diff.String())

	_, err = a.Add(other)
	require.ErrorIs(t, err, money.ErrAssetMismatch)

	_, err = a.Sub(otherDecimals)
	require.ErrorIs(t, err, money.ErrDecimalsMismatch)

	max := money.MustParse("92233720368547758.07", "USDT", 2)
	one := money.MustParse("0.01", "USDT", 2)
	_, err = max.Add(one)
	require.ErrorIs(t, err, money.ErrOverflow)

	min := money.MustParse("-92233720368547758.08", "USDT", 2)
	_, err = min.Sub(one)
	require.ErrorIs(t, err, money.ErrOverflow)

	negative := money.MustParse("-1.00", "USDT", 2)
	assert.Equal(t, "99.50", func() string { s, _ := a.Add(negative); return s.String() }())
}

func TestMulInt(t *testing.T) {
	t.Parallel()

	m := money.MustParse("10.25", "USDT", 2)

	got, err := m.MulInt(3)
	require.NoError(t, err)
	assert.Equal(t, "30.75", got.String())

	got, err = m.MulInt(0)
	require.NoError(t, err)
	assert.True(t, got.IsZero())

	got, err = m.MulInt(1)
	require.NoError(t, err)
	assert.Equal(t, m, got)

	got, err = m.MulInt(-1)
	require.NoError(t, err)
	assert.Equal(t, "-10.25", got.String())

	max := money.MustParse("92233720368547758.07", "USDT", 2)
	_, err = max.MulInt(2)
	require.ErrorIs(t, err, money.ErrOverflow)

	min := money.MustParse("-92233720368547758.08", "USDT", 2)
	_, err = min.MulInt(-1)
	require.ErrorIs(t, err, money.ErrOverflow)
}

func TestNegCmpIsZero(t *testing.T) {
	t.Parallel()

	m := money.MustParse("10.25", "USDT", 2)
	neg, err := m.Neg()
	require.NoError(t, err)
	assert.Equal(t, "-10.25", neg.String())

	_, err = money.MustParse("-92233720368547758.08", "USDT", 2).Neg()
	require.ErrorIs(t, err, money.ErrOverflow)

	other := money.MustParse("10.25", "USDT", 2)
	cmp, err := m.Cmp(other)
	require.NoError(t, err)
	assert.Equal(t, 0, cmp)

	cmp, err = m.Cmp(neg)
	require.NoError(t, err)
	assert.Equal(t, 1, cmp)

	cmp, err = neg.Cmp(m)
	require.NoError(t, err)
	assert.Equal(t, -1, cmp)

	_, err = m.Cmp(money.MustParse("1", "BTC", 2))
	require.ErrorIs(t, err, money.ErrAssetMismatch)

	assert.True(t, money.MustParse("0", "USDT", 2).IsZero())
	assert.False(t, m.IsZero())
}

func TestMustParsePanics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() { money.MustParse("bad", "USDT", 2) })
}
