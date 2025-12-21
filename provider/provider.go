package provider

import (
	"context"

	"wingman/models"
)

type InferenceProvider interface {
	RunInference(ctx context.Context, messages []models.WingmanMessage, instructions string) (*models.WingmanMessageResponse, error)
}
