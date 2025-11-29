package provider

import (
	"context"
	"fmt"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, input any) (any, error)
}

type ProviderBuilder func(config map[string]any) (InferenceProvider, error)

type registry struct {
	providerBuilders map[string]ProviderBuilder
}

var wingmanRegistry = &registry{
	providerBuilders: make(map[string]ProviderBuilder),
}

func Register(name string, providerBuilder ProviderBuilder) {
	wingmanRegistry.providerBuilders[name] = providerBuilder
}

func CreateProvider(name string, config map[string]any) (InferenceProvider, error) {
	providerBuilder, ok := wingmanRegistry.providerBuilders[name]

	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	return providerBuilder(config)
}

func PrintProvidersInRegistry() {
	for name := range wingmanRegistry.providerBuilders {
		fmt.Println(name)
	}
}
