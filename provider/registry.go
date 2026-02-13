package provider

import (
	"fmt"
	"sync"
)

type AuthType string

const (
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth  AuthType = "oauth"
)

type ProviderMeta struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	AuthTypes []AuthType `json:"auth_types"`
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]ProviderMeta
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]ProviderMeta),
	}
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
	_, providerExists := r.providers[name]
	return providerExists
}

var defaultRegistry = NewRegistry()

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func Register(meta ProviderMeta) {
	defaultRegistry.Register(meta)
}

func Get(name string) (ProviderMeta, error) {
	return defaultRegistry.Get(name)
}

func List() []ProviderMeta {
	return defaultRegistry.List()
}

func IsValid(name string) bool {
	return defaultRegistry.IsValid(name)
}
