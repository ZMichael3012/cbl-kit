# CBL-17.21 - Asset and network catalog: runtime model and validation (cbl-kit)

Task: CBL-17.21 in `CBL-004` (E17, W5-SCALE), depends on 17.1 (done).
Repositories: `cbl-protos` (the versioned artifact and its JSON Schema, spec in that
repository) and `cbl-kit` (this spec: the typed model, the strict parser, the validation
and the lookups used by services). Status: in progress.

## 1. Problem and scope

Provider adapters, the quote service, the ledger and the reporting facts use asset codes
(`BTC`, `USDT`) and network codes (`usdt_trc20`, `lightning`) that until now exist only in
code and tests. Provider APIs use their own codes (`usdttrc20` for NOWPayments, `TRON` as a
CoinGate platform, `BTC_BTC` for BTCPay). Without one catalog the same asset can be written
with different decimals or mapped to different provider codes in different services, and
money invariants (int64 minor units plus decimals) break silently.

In scope: a Go package that parses the canonical artifact, validates every invariant and
answers the lookup questions consumers need, plus the test that the canonical artifact is
accepted. Out of scope: changing provider adapters (their API codes stay as they are until a
consumer task wires the catalog), quote/ledger/reporting wiring (17.15 and later), the JSON
Schema artifact itself (it lives in `cbl-protos`).

## 2. Architecture

```
cbl-kit/catalog/
  catalog.go        # types, ErrInvalidCatalog, Parse, Validate, lookups
  catalog_test.go   # unit matrix on fixtures, canonical artifact cross-check
```

Public API:

```go
type ProviderCode struct {
    Asset   string // provider currency code, including a combined network suffix when the provider uses one
    Network string // optional separate provider platform/network code
}

type Network struct {
    Code          string
    Name          string
    Kind          string // native, token, layer
    Chain         string
    Status        string // active, deprecated
    ProviderCodes map[string]ProviderCode
}

type Asset struct {
    Code     string
    Name     string
    Decimals int
    Status   string
    Networks []Network
}

type Catalog struct {
    SchemaVersion  int
    CatalogVersion int
    UpdatedAt      string
    Assets         []Asset
}

func Parse(data []byte) (*Catalog, error)
func (c *Catalog) Validate() error
func (c *Catalog) Asset(code string) (Asset, bool)
func (c *Catalog) Network(code string) (network Network, asset Asset, ok bool)
func (c *Catalog) ProviderCode(provider, assetCode, networkCode string) (ProviderCode, bool)
func (c *Catalog) ActiveAssets() []Asset
```

Design decisions:

- The artifact is JSON, strict decoding (`DisallowUnknownFields`): a typo in a field name
  must fail loudly, never silently drop data.
- `Parse` reads and validates; `Validate` is exported because consumers may carry the
  artifact through config and validate at startup.
- Lookups are exact and case-sensitive: canonical codes are lowercase (`usdt_trc20`) and
  asset codes uppercase (`USDT`); a lenient lookup would hide inconsistent call sites.
- `ActiveAssets` filters the deprecated status; every other lookup returns deprecated
  entries so an old payment can still be resolved.
- Money decimals are the canonical per-asset value; networks never override them (a token
  on two chains keeps the issuer decimals, for example USDT 6 on TRON and Ethereum).

## 3. Validation rules

V1. `schema_version` is 1; `catalog_version` is at least 1; `updated_at` is a `YYYY-MM-DD`
    date.
V2. Asset codes match `^[A-Z0-9]{2,10}$` and are unique; names are trimmed, non-empty and
    at most 100 bytes; decimals are 0..18; status is active or deprecated; at least one
    network per asset.
V3. Network codes match `^[a-z0-9_]{2,32}$` and are globally unique; names non-empty; kind
    is native, token or layer; chain matches `^[a-z0-9_]{2,32}$`; status is active or
    deprecated.
V4. Provider keys are the known providers (coingate, nowpayments, bitpay, btcpay);
    provider asset codes match `^[A-Za-z0-9_.:-]{1,32}$`; provider network codes are empty
    or match the same pattern; one provider asset code maps to exactly one canonical asset
    (copy-paste safety); a non-empty provider network code combined with the asset code is
    unique; an active network has at least one provider code. CoinGate and NOWPayments use
    a combined code (`USDT_TRC20`, `usdttrc20`) in `asset`; BitPay and BTCPay are
    chain-agnostic, so their `asset` code may legitimately repeat across the networks of
    one canonical asset (`USDT` on TRON and Ethereum).
V5. Each known provider appears at least once; at least one asset is active.
V6. Whitespace around any code, name or provider code is rejected (trimmed comparison), so
    values can be compared byte-exactly on the wire.
V7. An empty artifact (no assets) is invalid.
V8. Every active asset has at least one active network.
V9. The artifact is at most 1 MiB; the parser fails closed on a larger input.

## 4. Data flow

The artifact is released from `cbl-protos` (`catalog/assets.json`, version 1). A service
embeds or mounts the file, calls `catalog.Parse` at startup, fails closed on an error, and
uses `Asset`/`Network`/`ProviderCode` to translate between the canonical codes and the
provider API codes. The kit package performs no I/O and has no configuration.

## 5. Error taxonomy

`ErrInvalidCatalog` (sentinel) with a context: `catalog: invalid asset catalog: ...`.
Parse errors from JSON are wrapped with `%w`. No panics.

## 6. Test plan

Unit matrix (fixtures as JSON in the test):

| ID | Case | Expectation |
| :-- | :-- | :-- |
| K1 | valid fixture with two assets, two networks, all providers | Parse succeeds, lookups resolve |
| K2 | unknown JSON field | error |
| K3 | schema_version 2 | error |
| K4 | catalog_version 0 | error |
| K5 | updated_at `20.10.2026` | error |
| K6 | duplicate asset code | error |
| K7 | asset code `usdt` (lowercase) | error |
| K8 | decimals -1 and 19 | error |
| K9 | asset without networks | error |
| K10 | duplicate network code in another asset | error |
| K11 | unknown provider key | error |
| K12 | provider asset code with a trailing space | error |
| K13 | one provider asset code mapped to two canonical assets | error |
| K14 | active network without provider codes | error |
| K15 | empty assets | error |
| K16 | deprecated asset filtered by ActiveAssets, still resolvable by Asset | both |
| K17 | Network lookup returns the owning asset | both |
| K18 | ProviderCode for an absent provider | false |
| K19 | canonical artifact cross-check (skips when `../cbl-protos` is absent) | Parse succeeds |
| K22 | non-empty provider network code repeated across networks | error |
| K23 | artifact larger than 1 MiB | error |
| K24 | chain-agnostic provider code (BitPay `USDT`) reused across two networks of one asset | Parse succeeds |

Integration: not applicable, the package has no I/O.
E2E: a library has no stand. The closest end-to-end check is K19: the parser consumes the
actual artifact released by `cbl-protos`; the service stands are unaffected because no
adapter is wired yet.

## 7. Red evidence

Recorded 20.10.2026 before the implementation: `go test ./catalog` failed with
`no non-test Go files in .../cbl-kit/catalog` (the tests referenced `catalog.Parse`,
`Asset`, `Network` and `ProviderCode`, none of which existed). The protos side was red
with `open assets.schema.json` and `open assets.json`: the artifact and the schema did
not exist yet.

## 8. Results and review closure (20.10.2026)

- Unit: the full matrix K1 - K24 green; coverage of the package 89.8%.
- Cross-repo: K19 parses the actual `cbl-protos/catalog/assets.json` artifact.
- Gates: `go test ./...` green, `go vet ./...` clean, golangci-lint 0 issues, em dash check green.
- Review pass 1 (no-context): the spec claimed services consume the catalog now; no
  adapter is wired until 17.15 and later. The data-flow section states the consumption
  path and the out-of-scope boundary.
- Review pass 2 (architecture, blocking): the initial (provider, asset, network) uniqueness
  rule rejected the canonical artifact because BitPay and BTCPay are chain-agnostic, and
  the draft provider codes invented platform semantics the adapters do not send. Closed:
  the provider codes follow the adapter wire values; the rule is now one provider asset
 code maps to one canonical asset plus uniqueness of a non-empty provider network code;
  K24 proves the chain-agnostic reuse and K13/K22 prove the two rejections.
- Review pass 3 (with-context): the invariants are mirrored in the protos test; T-KK10
  records the dependency-free contracts constraint and the K19 cross-check.
- Review pass 4 (security): the parser had no input bound; V9/K23 add the 1 MiB cap.
  No secrets, no float, codes are byte-exact (trimmed values are rejected).
- NIT: the spec matrix IDs drifted from the tests after the fixes; the matrix now lists
  K22 (provider network duplicate), K23 (oversized artifact) and K24 (chain-agnostic code).
