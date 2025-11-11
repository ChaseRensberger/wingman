package main

import (
	"context"
	"fmt"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, input any) (any, error)
}

func CreateInferenceProvider(name string, config map[string]any) (InferenceProvider, error) {
	switch name {
	case "anthropic":
		return CreateAnthropicClient(config)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
