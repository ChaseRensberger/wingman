package provider

import (
	"context"

	"wingman/models"
)

type Provider interface {
	RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error)
	RunInferenceStream(ctx context.Context, req models.WingmanInferenceRequest) (Stream, error)
}

type Stream interface {
	Next() bool
	Event() models.StreamEvent
	Err() error
	Close() error
	Response() *models.WingmanInferenceResponse
}
