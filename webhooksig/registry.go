package webhooksig

import (
	"fmt"
	"strings"
	"sync"
)

// Registry maps provider identifiers to verification strategies.
type Registry struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{strategies: make(map[string]Strategy)}
}

// Register adds the strategy of a provider; an empty provider, a nil strategy
// or a duplicate registration is an error, because duplicates hide configuration
// mistakes (a reload builds a fresh registry).
func (r *Registry) Register(provider string, strategy Strategy) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("%w: empty provider", ErrMalformed)
	}
	if strategy == nil {
		return fmt.Errorf("%w: nil strategy for %q", ErrMalformed, provider)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.strategies[provider]; exists {
		return fmt.Errorf("%w: provider %q is already registered", ErrMalformed, provider)
	}
	r.strategies[provider] = strategy
	return nil
}

// Lookup returns the strategy registered for the provider.
func (r *Registry) Lookup(provider string) (Strategy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	strategy, ok := r.strategies[provider]
	return strategy, ok
}
