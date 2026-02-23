package provider

import (
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/core"
)

// AuthType describes the kind of credential a provider requires.
type AuthType string

const (
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth  AuthType = "oauth"
)

// ProviderFactory is a function that constructs a Provider from an options map.
// The options map is the same map stored on the agent — it should contain at
// minimum "model", plus any inference parameters (max_tokens, temperature, etc.)
// and auth credentials (api_key, etc.) that the provider needs.
//
// Providers register their factory in an init() function so that importing the
// provider package is sufficient to make it available via provider.New().
type ProviderFactory func(opts map[string]any) (core.Provider, error)

// ProviderMeta describes a provider for use by the registry and the server's
// auth management endpoints.
type ProviderMeta struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	AuthTypes []AuthType      `json:"auth_types"`
	Factory   ProviderFactory `json:"-"` // not serialised; used internally
}

// Registry holds all registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]ProviderMeta
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]ProviderMeta),
	}
}

// Register adds or replaces a provider entry.
func (r *Registry) Register(meta ProviderMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[meta.ID] = meta
}

// Get returns the ProviderMeta for the given provider ID.
func (r *Registry) Get(name string) (ProviderMeta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.providers[name]
	if !ok {
		return ProviderMeta{}, fmt.Errorf("provider not found: %s", name)
	}
	return meta, nil
}

// New constructs a Provider from the registry using the registered factory.
// opts is forwarded directly to the factory; it should contain at minimum
// "model" plus any provider-specific keys (api_key, base_url, etc.) and
// inference parameters (max_tokens, temperature, etc.).
func (r *Registry) New(providerID string, opts map[string]any) (core.Provider, error) {
	meta, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}
	if meta.Factory == nil {
		return nil, fmt.Errorf("provider %q has no factory registered", providerID)
	}
	p, err := meta.Factory(opts)
	if err != nil {
		return nil, fmt.Errorf("provider %q factory error: %w", providerID, err)
	}
	if p == nil {
		return nil, fmt.Errorf("provider %q factory returned nil (missing auth?)", providerID)
	}
	return p, nil
}

// List returns all registered providers. The Factory field is not included in
// JSON serialisation so this is safe to return in HTTP responses.
func (r *Registry) List() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProviderMeta, 0, len(r.providers))
	for _, meta := range r.providers {
		result = append(result, meta)
	}
	return result
}

// IsValid reports whether a provider with the given ID is registered.
func (r *Registry) IsValid(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[name]
	return ok
}

// ============================================================
//  Package-level singleton registry and convenience functions
// ============================================================

var defaultRegistry = NewRegistry()

// DefaultRegistry returns the package-level default registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Register adds a provider to the default registry. Provider packages call
// this in their init() functions.
func Register(meta ProviderMeta) {
	defaultRegistry.Register(meta)
}

// Get looks up a provider in the default registry.
func Get(name string) (ProviderMeta, error) {
	return defaultRegistry.Get(name)
}

// New constructs a Provider using the default registry. This is the primary
// way to instantiate a provider without knowing the concrete type at compile
// time. Provider packages that want to be usable via New() must register a
// Factory in their init() function.
//
// Example:
//
//	import _ "github.com/chaserensberger/wingman/provider/anthropic"
//
//	p, err := provider.New("anthropic", map[string]any{
//	    "model":      "claude-opus-4-6",
//	    "max_tokens": 4096,
//	    "api_key":    "sk-...",
//	})
func New(providerID string, opts map[string]any) (core.Provider, error) {
	return defaultRegistry.New(providerID, opts)
}

// List returns all providers in the default registry.
func List() []ProviderMeta {
	return defaultRegistry.List()
}

// IsValid reports whether the given provider ID is in the default registry.
func IsValid(name string) bool {
	return defaultRegistry.IsValid(name)
}
