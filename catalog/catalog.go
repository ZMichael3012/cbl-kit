// Package catalog parses and validates the canonical asset and network catalog
// released by cbl-protos (CBL-17.21).
package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// SchemaVersionV1 is the supported artifact schema version.
const SchemaVersionV1 = 1

// Status values of an asset or a network.
const (
	StatusActive     = "active"
	StatusDeprecated = "deprecated"
)

// Network kinds.
const (
	NetworkKindNative = "native"
	NetworkKindToken  = "token"
	NetworkKindLayer  = "layer"
)

// maxNameLen bounds an asset or network display name.
const maxNameLen = 100

// maxDecimals bounds the canonical minor-unit scale (money invariant).
const maxDecimals = 18

// maxArtifactBytes bounds the artifact read from a trusted deployment source;
// the guard turns an accidental huge file into a startup failure, not memory
// pressure.
const maxArtifactBytes = 1 << 20

// ErrInvalidCatalog reports an artifact that violates the catalog contract.
var ErrInvalidCatalog = errors.New("catalog: invalid asset catalog")

var (
	assetCodePattern    = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)
	networkCodePattern  = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)
	providerCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,32}$`)
)

// knownProviders is the closed set of provider identifiers (SPI v1 manifests).
var knownProviders = map[string]struct{}{
	"coingate":    {},
	"nowpayments": {},
	"bitpay":      {},
	"btcpay":      {},
}

// ProviderCode is the provider wire code of one canonical network: the asset
// (currency) code and the optional platform or network code.
type ProviderCode struct {
	// Asset sets the provider currency or asset code used on the wire.
	Asset string `json:"asset"`
	// Network sets the optional provider platform or network code.
	Network string `json:"network,omitempty"`
}

// Network is one canonical network of an asset.
type Network struct {
	// Code is the canonical lowercase network code used in routing.
	Code string `json:"code"`
	// Name is the operator-facing network name.
	Name string `json:"name"`
	// Kind is native, token or layer.
	Kind string `json:"kind"`
	// Chain is the chain identifier shared by native and token networks.
	Chain string `json:"chain"`
	// Status is active or deprecated.
	Status string `json:"status"`
	// ProviderCodes maps a provider id to its wire code.
	ProviderCodes map[string]ProviderCode `json:"provider_codes"`
}

// Asset is one canonical crypto asset and its networks.
type Asset struct {
	// Code is the canonical uppercase asset code.
	Code string `json:"code"`
	// Name is the operator-facing asset name.
	Name string `json:"name"`
	// Decimals is the canonical minor-unit scale.
	Decimals int `json:"decimals"`
	// Status is active or deprecated.
	Status string `json:"status"`
	// Networks lists the networks of the asset.
	Networks []Network `json:"networks"`
}

// Catalog is the parsed asset and network catalog artifact.
type Catalog struct {
	// SchemaVersion is the artifact schema version.
	SchemaVersion int `json:"schema_version"`
	// CatalogVersion is the data revision, monotonically increasing.
	CatalogVersion int `json:"catalog_version"`
	// UpdatedAt is the date of the revision (YYYY-MM-DD).
	UpdatedAt string `json:"updated_at"`
	// Assets lists the assets of the catalog.
	Assets []Asset `json:"assets"`
}

// Parse decodes and validates a catalog artifact. Unknown fields, trailing
// data and every contract violation fail with ErrInvalidCatalog.
func Parse(data []byte) (*Catalog, error) {
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("%w: artifact is larger than %d bytes", ErrInvalidCatalog, maxArtifactBytes)
	}
	var parsed Catalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidCatalog, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing data after the catalog object", ErrInvalidCatalog)
	}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// Validate checks the catalog contract invariants.
func (c *Catalog) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil catalog", ErrInvalidCatalog)
	}
	if c.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidCatalog, c.SchemaVersion)
	}
	if c.CatalogVersion < 1 {
		return fmt.Errorf("%w: catalog_version %d is not positive", ErrInvalidCatalog, c.CatalogVersion)
	}
	if _, err := time.Parse("2006-01-02", c.UpdatedAt); err != nil {
		return fmt.Errorf("%w: updated_at %q is not a YYYY-MM-DD date", ErrInvalidCatalog, c.UpdatedAt)
	}
	if len(c.Assets) == 0 {
		return fmt.Errorf("%w: no assets", ErrInvalidCatalog)
	}

	seenAssets := make(map[string]struct{}, len(c.Assets))
	seenNetworks := make(map[string]struct{})
	providerAssets := make(map[string]string)
	providerNetworks := make(map[string]struct{})
	seenProviders := make(map[string]struct{}, len(knownProviders))
	activeAssets := 0
	for _, asset := range c.Assets {
		if err := validateAsset(asset, seenAssets, seenNetworks, providerAssets, providerNetworks, seenProviders); err != nil {
			return err
		}
		if asset.Status == StatusActive {
			activeAssets++
			if !hasActiveNetwork(asset) {
				return fmt.Errorf("%w: active asset %q has no active network", ErrInvalidCatalog, asset.Code)
			}
		}
	}
	if activeAssets == 0 {
		return fmt.Errorf("%w: no active asset", ErrInvalidCatalog)
	}
	for provider := range knownProviders {
		if _, ok := seenProviders[provider]; !ok {
			return fmt.Errorf("%w: provider %q is not mapped by any network", ErrInvalidCatalog, provider)
		}
	}
	return nil
}

// validateAsset checks one asset and registers its codes in the catalog-wide
// uniqueness sets.
func validateAsset(asset Asset, seenAssets, seenNetworks map[string]struct{}, providerAssets map[string]string, providerNetworks, seenProviders map[string]struct{}) error {
	if !assetCodePattern.MatchString(asset.Code) {
		return fmt.Errorf("%w: invalid asset code %q", ErrInvalidCatalog, asset.Code)
	}
	if _, duplicate := seenAssets[asset.Code]; duplicate {
		return fmt.Errorf("%w: duplicate asset code %q", ErrInvalidCatalog, asset.Code)
	}
	seenAssets[asset.Code] = struct{}{}
	if err := validateText("asset", asset.Code, asset.Name); err != nil {
		return err
	}
	if asset.Decimals < 0 || asset.Decimals > maxDecimals {
		return fmt.Errorf("%w: asset %q decimals %d out of range", ErrInvalidCatalog, asset.Code, asset.Decimals)
	}
	if asset.Status != StatusActive && asset.Status != StatusDeprecated {
		return fmt.Errorf("%w: asset %q has unknown status %q", ErrInvalidCatalog, asset.Code, asset.Status)
	}
	if len(asset.Networks) == 0 {
		return fmt.Errorf("%w: asset %q has no networks", ErrInvalidCatalog, asset.Code)
	}
	for _, network := range asset.Networks {
		if err := validateNetwork(asset.Code, network, seenNetworks, providerAssets, providerNetworks, seenProviders); err != nil {
			return err
		}
	}
	return nil
}

// validateNetwork checks one network and its provider codes.
func validateNetwork(assetCode string, network Network, seenNetworks map[string]struct{}, providerAssets map[string]string, providerNetworks, seenProviders map[string]struct{}) error {
	if !networkCodePattern.MatchString(network.Code) {
		return fmt.Errorf("%w: invalid network code %q", ErrInvalidCatalog, network.Code)
	}
	if _, duplicate := seenNetworks[network.Code]; duplicate {
		return fmt.Errorf("%w: duplicate network code %q", ErrInvalidCatalog, network.Code)
	}
	seenNetworks[network.Code] = struct{}{}
	if err := validateText("network", network.Code, network.Name); err != nil {
		return err
	}
	switch network.Kind {
	case NetworkKindNative, NetworkKindToken, NetworkKindLayer:
	default:
		return fmt.Errorf("%w: network %q has unknown kind %q", ErrInvalidCatalog, network.Code, network.Kind)
	}
	if !networkCodePattern.MatchString(network.Chain) {
		return fmt.Errorf("%w: network %q has invalid chain %q", ErrInvalidCatalog, network.Code, network.Chain)
	}
	if network.Status != StatusActive && network.Status != StatusDeprecated {
		return fmt.Errorf("%w: network %q has unknown status %q", ErrInvalidCatalog, network.Code, network.Status)
	}
	if network.Status == StatusActive && len(network.ProviderCodes) == 0 {
		return fmt.Errorf("%w: active network %q has no provider codes", ErrInvalidCatalog, network.Code)
	}
	for provider, code := range network.ProviderCodes {
		if _, known := knownProviders[provider]; !known {
			return fmt.Errorf("%w: network %q maps unknown provider %q", ErrInvalidCatalog, network.Code, provider)
		}
		if !providerCodePattern.MatchString(code.Asset) {
			return fmt.Errorf("%w: network %q provider %q has invalid asset code %q", ErrInvalidCatalog, network.Code, provider, code.Asset)
		}
		if code.Network != "" && !providerCodePattern.MatchString(code.Network) {
			return fmt.Errorf("%w: network %q provider %q has invalid network code %q", ErrInvalidCatalog, network.Code, provider, code.Network)
		}
		if err := validateText("provider code", provider, code.Asset); err != nil {
			return err
		}
		if code.Network != "" {
			if err := validateText("provider code", provider, code.Network); err != nil {
				return err
			}
		}
		// A provider asset code identifies exactly one canonical asset: a
		// copy-paste that maps, for example, the NOWPayments USDT code to ETH
		// must fail here, before it is used to price an invoice.
		key := provider + "\x00" + code.Asset
		if existing, ok := providerAssets[key]; ok && existing != assetCode {
			return fmt.Errorf("%w: network %q provider %q code %q maps both %q and %q", ErrInvalidCatalog, network.Code, provider, code.Asset, existing, assetCode)
		}
		providerAssets[key] = assetCode
		// A separate provider platform code, when set, must identify one
		// canonical network; chain-agnostic providers leave it empty, so the
		// same currency code may legitimately cover several networks.
		if code.Network != "" {
			networkKey := key + "\x00" + code.Network
			if _, duplicate := providerNetworks[networkKey]; duplicate {
				return fmt.Errorf("%w: network %q duplicates provider %q code %q/%q", ErrInvalidCatalog, network.Code, provider, code.Asset, code.Network)
			}
			providerNetworks[networkKey] = struct{}{}
		}
		seenProviders[provider] = struct{}{}
	}
	return nil
}

// validateText rejects empty or padded display text; codes are pattern checked
// separately, this guard also keeps them byte-exact for wire comparisons.
func validateText(kind, code, text string) error {
	if text == "" || strings.TrimSpace(text) != text {
		return fmt.Errorf("%w: %s %q has an empty or padded text %q", ErrInvalidCatalog, kind, code, text)
	}
	if len(text) > maxNameLen {
		return fmt.Errorf("%w: %s %q text is too long", ErrInvalidCatalog, kind, code)
	}
	return nil
}

// hasActiveNetwork reports whether the asset has at least one active network.
func hasActiveNetwork(asset Asset) bool {
	for _, network := range asset.Networks {
		if network.Status == StatusActive {
			return true
		}
	}
	return false
}

// Asset returns the asset with the canonical code. The returned value shares
// the catalog storage and must be treated as read-only.
func (c *Catalog) Asset(code string) (Asset, bool) {
	for _, asset := range c.Assets {
		if asset.Code == code {
			return asset, true
		}
	}
	return Asset{}, false
}

// Network returns the network with the canonical code and its owning asset.
// The returned values share the catalog storage and must be treated as
// read-only.
func (c *Catalog) Network(code string) (Network, Asset, bool) {
	for _, asset := range c.Assets {
		for _, network := range asset.Networks {
			if network.Code == code {
				return network, asset, true
			}
		}
	}
	return Network{}, Asset{}, false
}

// ProviderCode returns the wire code of the provider for the canonical asset
// and network. Deprecated entries stay resolvable so old payments can be
// explained.
func (c *Catalog) ProviderCode(provider, assetCode, networkCode string) (ProviderCode, bool) {
	for _, asset := range c.Assets {
		if asset.Code != assetCode {
			continue
		}
		for _, network := range asset.Networks {
			if network.Code != networkCode {
				continue
			}
			code, ok := network.ProviderCodes[provider]
			return code, ok
		}
	}
	return ProviderCode{}, false
}

// ActiveAssets returns the assets with the active status. The returned slice
// shares the catalog storage and must be treated as read-only.
func (c *Catalog) ActiveAssets() []Asset {
	active := make([]Asset, 0, len(c.Assets))
	for _, asset := range c.Assets {
		if asset.Status == StatusActive {
			active = append(active, asset)
		}
	}
	return active
}
