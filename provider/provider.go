package provider

import (
	"context"
	"fmt"
	"sync"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, input any) (any, error)
}

type ProviderFactory func(config map[string]any) (InferenceProvider, error)

type registry struct {
	mu                sync.RWMutex
	providerFactories map[string]ProviderFactory
}

var globalRegistry = &registry{
	providerFactories: make(map[string]ProviderFactory),
}

func Register(name string, factory ProviderFactory) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.providerFactories[name] = factory
}

func CreateProvider(name string, config map[string]any) (InferenceProvider, error) {
	globalRegistry.mu.RLock()
	factory, ok := globalRegistry.providerFactories[name]
	globalRegistry.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	return factory(config)
}
