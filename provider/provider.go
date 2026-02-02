package provider

import (
	"context"

	"wingman/models"
)

type Provider interface {
	RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error)
}
