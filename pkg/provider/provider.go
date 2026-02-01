package provider

import (
	"context"

	"wingman/pkg/models"
)

type Provider interface {
	RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error)
}
