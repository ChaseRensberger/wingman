package registry

import (
	"fmt"
)

type ProviderBuilder func(config map[string]any) (any, error)

type Registry struct {
	providerBuilders map[string]ProviderBuilder
}

var globalRegistry = &Registry{
	providerBuilders: make(map[string]ProviderBuilder),
}

func Register(name string, providerBuilder ProviderBuilder) {
	globalRegistry.providerBuilders[name] = providerBuilder
}

func GetBuilder(name string) (ProviderBuilder, error) {
	builder, ok := globalRegistry.providerBuilders[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return builder, nil
}

func ListProvidersInRegistry() []string {
	providers := make([]string, 0, len(globalRegistry.providerBuilders))
	for name := range globalRegistry.providerBuilders {
		providers = append(providers, name)
	}
	return providers
}
