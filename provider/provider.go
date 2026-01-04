package provider

import (
	"context"

	"wingman/models"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, messages []models.WingmanMessage, config models.WingmanConfig) (*models.WingmanMessageResponse, error)
}

type ProviderFactory func(config models.WingmanConfig) (InferenceProvider, error)
