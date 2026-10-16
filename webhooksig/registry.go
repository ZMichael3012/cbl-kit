package webhooksig

import (
	"fmt"
	"reflect"
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
// (including a typed nil) or a duplicate registration is an error, because
// duplicates hide configuration mistakes (a reload builds a fresh registry).
func (r *Registry) Register(provider string, strategy Strategy) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("%w: empty provider", ErrMalformed)
	}
	if isNilStrategy(strategy) {
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

// Lookup returns the strategy registered for the provider; the provider name is
// normalized the same way as in Register.
func (r *Registry) Lookup(provider string) (Strategy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	strategy, ok := r.strategies[strings.TrimSpace(provider)]
	return strategy, ok
}

// isNilStrategy reports a nil interface or a typed nil pointer, which would
// otherwise pass the interface comparison and panic on Verify.
func isNilStrategy(strategy Strategy) bool {
	if strategy == nil {
		return true
	}
	value := reflect.ValueOf(strategy)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return value.IsNil()
	default:
		return false
	}
}
