package provider

import (
	"context"
	"fmt"
	"wingman/models"
	"wingman/provider/registry"

	_ "wingman/provider/anthropic"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, messages []models.WingmanMessage) (*models.WingmanMessageResponse, error)
}

func GetProviderFromRegistry(name string, config map[string]any) (InferenceProvider, error) {
	builder, err := registry.GetBuilder(name)
	if err != nil {
		return nil, err
	}

	provider, err := builder(config)
	if err != nil {
		return nil, err
	}

	inferenceProvider, ok := provider.(InferenceProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not implement InferenceProvider", name)
	}

	return inferenceProvider, nil
}
