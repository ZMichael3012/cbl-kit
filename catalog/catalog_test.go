package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/catalog"
)

// baseCatalog is the minimal valid artifact: one asset, one network, every
// provider covered.
const baseCatalog = `{
  "schema_version": 1,
  "catalog_version": 1,
  "updated_at": "2026-10-20",
  "assets": [
    {
      "code": "USDT",
      "name": "Tether USD",
      "decimals": 6,
      "status": "active",
      "networks": [
        {
          "code": "usdt_trc20",
          "name": "Tether on TRON (TRC20)",
          "kind": "token",
          "chain": "tron",
          "status": "active",
          "provider_codes": {
            "coingate": {"asset": "USDT_TRC20"},
            "nowpayments": {"asset": "usdttrc20"},
            "bitpay": {"asset": "USDT"},
            "btcpay": {"asset": "USDT"}
          }
        }
      ]
    }
  ]
}`

// fixture replaces one fragment of the base artifact.
func fixture(t *testing.T, old, updated string) []byte {
	t.Helper()
	require.Contains(t, baseCatalog, old, "the fixture fragment exists")
	return []byte(strings.Replace(baseCatalog, old, updated, 1))
}

func TestParseValid(t *testing.T) {
	parsed, err := catalog.Parse([]byte(baseCatalog))
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.SchemaVersion)
	assert.Equal(t, 1, parsed.CatalogVersion)
	assert.Equal(t, "2026-10-20", parsed.UpdatedAt)

	asset, ok := parsed.Asset("USDT")
	require.True(t, ok)
	assert.Equal(t, 6, asset.Decimals)
	assert.Equal(t, "Tether USD", asset.Name)

	network, owner, ok := parsed.Network("usdt_trc20")
	require.True(t, ok)
	assert.Equal(t, "USDT", owner.Code)
	assert.Equal(t, "token", network.Kind)
	assert.Equal(t, "USDT_TRC20", network.ProviderCodes["coingate"].Asset)

	code, ok := parsed.ProviderCode("nowpayments", "USDT", "usdt_trc20")
	require.True(t, ok)
	assert.Equal(t, "usdttrc20", code.Asset)

	_, ok = parsed.ProviderCode("coingate", "USDT", "usdt_erc20")
	assert.False(t, ok, "an absent network has no provider code")
	_, ok = parsed.Asset("BTC")
	assert.False(t, ok)
	_, _, ok = parsed.Network("bitcoin")
	assert.False(t, ok)
}

func TestParseChainAgnosticProviderCodes(t *testing.T) {
	data := []byte(`{
	  "schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20",
	  "assets": [
	    {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
	      {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "active",
	       "provider_codes": {"coingate": {"asset": "USDT_TRC20"}, "nowpayments": {"asset": "usdttrc20"},
	                          "bitpay": {"asset": "USDT"}, "btcpay": {"asset": "USDT"}}},
	      {"code": "usdt_erc20", "name": "Tether on Ethereum", "kind": "token", "chain": "ethereum", "status": "active",
	       "provider_codes": {"coingate": {"asset": "USDT_ERC20"}, "nowpayments": {"asset": "usdterc20"},
	                          "bitpay": {"asset": "USDT"}, "btcpay": {"asset": "USDT"}}}]}
	  ]
	}`)
	parsed, err := catalog.Parse(data)
	require.NoError(t, err, "BitPay and BTCPay are chain-agnostic: the same currency code may cover two networks")

	bitpay, ok := parsed.ProviderCode("bitpay", "USDT", "usdt_erc20")
	require.True(t, ok)
	assert.Equal(t, "USDT", bitpay.Asset)
	assert.Empty(t, bitpay.Network)
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		id   string
		data []byte
	}{
		{"K2 unknown field", fixture(t, `"schema_version": 1,`, `"schema_version": 1, "extra": true,`)},
		{"K3 schema version", fixture(t, `"schema_version": 1,`, `"schema_version": 2,`)},
		{"K4 catalog version", fixture(t, `"catalog_version": 1,`, `"catalog_version": 0,`)},
		{"K5 updated_at format", fixture(t, `"updated_at": "2026-10-20"`, `"updated_at": "20.10.2026"`)},
		{"K6 duplicate asset", []byte(strings.Replace(baseCatalog, `"assets": [`, `"assets": [`+`
    {"code": "USDT", "name": "Tether again", "decimals": 6, "status": "active", "networks": [
      {"code": "usdt_erc20", "name": "Tether on Ethereum", "kind": "token", "chain": "ethereum", "status": "active",
       "provider_codes": {"coingate": {"asset": "USDT_ERC20"}}}]},`, 1))},
		{"K7 lowercase asset", fixture(t, `"code": "USDT"`, `"code": "usdt"`)},
		{"K8 negative decimals", fixture(t, `"decimals": 6`, `"decimals": -1`)},
		{"K8 large decimals", fixture(t, `"decimals": 6`, `"decimals": 19`)},
		{"K9 no networks", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": []}]}`)},
		{"K10 duplicate network", []byte(strings.Replace(baseCatalog, `"assets": [`, `"assets": [`+`
    {"code": "BTC", "name": "Bitcoin", "decimals": 8, "status": "active", "networks": [
      {"code": "usdt_trc20", "name": "Bitcoin on TRON", "kind": "native", "chain": "bitcoin", "status": "active",
       "provider_codes": {"bitpay": {"asset": "BTC"}}}]},`, 1))},
		{"K11 unknown provider", fixture(t, `"coingate": {"asset": "USDT_TRC20"}`, `"kraken": {"asset": "USDT"}`)},
		{"K12 trailing space", fixture(t, `"asset": "usdttrc20"`, `"asset": "usdttrc20 "`)},
		{"K13 provider code maps two assets", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "active",
		     "provider_codes": {"coingate": {"asset": "USDT_TRC20"},
		                        "nowpayments": {"asset": "usdttrc20"},
		                        "bitpay": {"asset": "USDT"},
		                        "btcpay": {"asset": "USDT"}}}]},
		  {"code": "USDC", "name": "USD Coin", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdc_erc20", "name": "USD Coin on Ethereum", "kind": "token", "chain": "ethereum", "status": "active",
		     "provider_codes": {"coingate": {"asset": "USDT_TRC20"}}}]}]}`)},
		{"K14 no provider codes", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "active",
		     "provider_codes": {}}]}]}`)},
		{"K15 empty assets", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": []}`)},
		{"K16 invalid JSON", []byte(`{"schema_version": 1,`)},
		{"K17 empty name", fixture(t, `"name": "Tether USD"`, `"name": " "`)},
		{"K18 invalid kind", fixture(t, `"kind": "token"`, `"kind": "wrapped"`)},
		{"K19 invalid date", fixture(t, `"updated_at": "2026-10-20"`, `"updated_at": "2026-13-40"`)},
		{"K20 missing provider coverage", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "active",
		     "provider_codes": {"coingate": {"asset": "USDT_TRC20"}}}]}]}`)},
		{"K21 active asset without active network", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "deprecated",
		     "provider_codes": {"coingate": {"asset": "USDT_TRC20"},
		                        "nowpayments": {"asset": "usdttrc20"},
		                        "bitpay": {"asset": "USDT"},
		                        "btcpay": {"asset": "USDT"}}}]}]}`)},
		{"K22 duplicate provider network code", []byte(`{"schema_version": 1, "catalog_version": 1, "updated_at": "2026-10-20", "assets": [
		  {"code": "USDT", "name": "Tether USD", "decimals": 6, "status": "active", "networks": [
		    {"code": "usdt_trc20", "name": "Tether on TRON", "kind": "token", "chain": "tron", "status": "active",
		     "provider_codes": {"coingate": {"asset": "USDT", "network": "TRON"},
		                        "nowpayments": {"asset": "usdttrc20"},
		                        "bitpay": {"asset": "USDT"},
		                        "btcpay": {"asset": "USDT"}}},
		    {"code": "usdt_erc20", "name": "Tether on Ethereum", "kind": "token", "chain": "ethereum", "status": "active",
		     "provider_codes": {"coingate": {"asset": "USDT", "network": "TRON"}}}]}]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			_, err := catalog.Parse(tc.data)
			require.Error(t, err)
			assert.ErrorIs(t, err, catalog.ErrInvalidCatalog)
		})
	}
}

func TestParseRejectsOversizedArtifact(t *testing.T) {
	data := []byte(strings.Repeat(" ", 1<<20) + baseCatalog)
	_, err := catalog.Parse(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, catalog.ErrInvalidCatalog)
}

func TestStatusFilteringAndLookups(t *testing.T) {
	data := []byte(`{
	  "schema_version": 1, "catalog_version": 2, "updated_at": "2026-10-21",
	  "assets": [
	    {"code": "BTC", "name": "Bitcoin", "decimals": 8, "status": "deprecated", "networks": [
	      {"code": "bitcoin", "name": "Bitcoin", "kind": "native", "chain": "bitcoin", "status": "deprecated",
	       "provider_codes": {"btcpay": {"asset": "BTC"}}}]},
	    {"code": "ETH", "name": "Ether", "decimals": 18, "status": "active", "networks": [
	      {"code": "ethereum", "name": "Ethereum", "kind": "native", "chain": "ethereum", "status": "active",
	       "provider_codes": {"btcpay": {"asset": "ETH"},
	                          "coingate": {"asset": "ETH"},
	                          "nowpayments": {"asset": "eth"},
	                          "bitpay": {"asset": "ETH"}}}]}
	  ]
	}`)
	parsed, err := catalog.Parse(data)
	require.NoError(t, err)

	active := parsed.ActiveAssets()
	require.Len(t, active, 1)
	assert.Equal(t, "ETH", active[0].Code)

	deprecated, ok := parsed.Asset("BTC")
	require.True(t, ok, "a deprecated asset stays resolvable")
	assert.Equal(t, "deprecated", deprecated.Status)

	network, owner, ok := parsed.Network("ethereum")
	require.True(t, ok)
	assert.Equal(t, "ETH", owner.Code)
	assert.Equal(t, "ETH", network.ProviderCodes["btcpay"].Asset)
}

func TestParseCanonicalArtifact(t *testing.T) {
	path := filepath.Join("..", "..", "cbl-protos", "catalog", "assets.json")
	data, err := os.ReadFile(path) //nolint:gosec // G304: fixed workspace-relative path to the sibling repository
	if os.IsNotExist(err) {
		t.Skip("the sibling cbl-protos checkout is absent")
	}
	require.NoError(t, err)

	parsed, err := catalog.Parse(data)
	require.NoError(t, err)
	assert.NotEmpty(t, parsed.ActiveAssets())
}
