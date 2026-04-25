// Package provider is the small registry that maps a provider id (e.g.
// "anthropic", "ollama") to a factory that constructs a wingmodels.Model.
//
// Providers register themselves at init via Register; cmd/wingman blank-
// imports each provider package to trigger registration. The wingagent
// session/server layers then look up providers by id and call New(opts) to
// build a Model bound to a specific model id.
package provider

import (
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/wingmodels"
)

// AuthType describes how a provider authenticates. Only api_key is exercised
// in v0.1; oauth is reserved for future provider integrations (Anthropic
// Claude.ai, OpenAI ChatGPT).
type AuthType string

const (
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth  AuthType = "oauth"
)

// ProviderFactory builds a wingmodels.Model from an opaque options bag.
// Returning a nil Model with a nil error is a contract violation; New rejects
// it.
type ProviderFactory func(opts map[string]any) (wingmodels.Model, error)

// ProviderMeta is the registry entry for one provider.
type ProviderMeta struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	AuthTypes []AuthType      `json:"auth_types"`
	Factory   ProviderFactory `json:"-"`
}

// Registry is a concurrency-safe map of provider id -> meta.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]ProviderMeta
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]ProviderMeta)}
}

func (r *Registry) Register(meta ProviderMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[meta.ID] = meta
}

func (r *Registry) Get(name string) (ProviderMeta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.providers[name]
	if !ok {
		return ProviderMeta{}, fmt.Errorf("provider not found: %s", name)
	}
	return meta, nil
}

// New constructs a Model from the named provider's factory.
func (r *Registry) New(providerID string, opts map[string]any) (wingmodels.Model, error) {
	meta, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}
	if meta.Factory == nil {
		return nil, fmt.Errorf("provider %q has no factory registered", providerID)
	}
	m, err := meta.Factory(opts)
	if err != nil {
		return nil, fmt.Errorf("provider %q factory error: %w", providerID, err)
	}
	if m == nil {
		return nil, fmt.Errorf("provider %q factory returned nil (missing auth?)", providerID)
	}
	return m, nil
}

func (r *Registry) List() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProviderMeta, 0, len(r.providers))
	for _, meta := range r.providers {
		result = append(result, meta)
	}
	return result
}

func (r *Registry) IsValid(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[name]
	return ok
}

var defaultRegistry = NewRegistry()

func DefaultRegistry() *Registry             { return defaultRegistry }
func Register(meta ProviderMeta)             { defaultRegistry.Register(meta) }
func Get(name string) (ProviderMeta, error)  { return defaultRegistry.Get(name) }
func New(providerID string, opts map[string]any) (wingmodels.Model, error) {
	return defaultRegistry.New(providerID, opts)
}
func List() []ProviderMeta { return defaultRegistry.List() }
func IsValid(name string) bool { return defaultRegistry.IsValid(name) }
