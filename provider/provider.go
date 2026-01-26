package provider

import (
	"context"

	"wingman/models"
)

type Provider interface {
	RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error)
	StreamInference(ctx context.Context, req models.WingmanInferenceRequest) (<-chan StreamEvent, error)
}

type StreamEventType string

const (
	StreamEventToken    StreamEventType = "token"
	StreamEventToolCall StreamEventType = "tool_call"
	StreamEventUsage    StreamEventType = "usage"
	StreamEventDone     StreamEventType = "done"
	StreamEventError    StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType
	Content string
	Delta   any
	Usage   *models.WingmanUsage
	Error   error
}
