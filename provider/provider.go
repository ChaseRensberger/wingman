package provider

import (
	"context"

	"github.com/chaserensberger/wingman/models"
)

type Provider interface {
	RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error)
	StreamInference(ctx context.Context, req models.WingmanInferenceRequest) (Stream, error)
}

type Stream interface {
	Next() bool
	Event() models.StreamEvent
	Err() error
	Close() error
	Response() *models.WingmanInferenceResponse
}
