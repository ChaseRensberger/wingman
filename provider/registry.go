package provider

type ProviderRegistry struct {
	providers map[string]InferenceProvider
}

func NewHandlerRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]InferenceProvider),
	}
}

func (r *ProviderRegistry) Register(name string, provider InferenceProvider) {
	r.providers[name] = provider
}
