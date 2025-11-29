package session

import (
	"context"

	"wingman/models"
	"wingman/provider"
)

type Session struct {
	inferenceProvider provider.InferenceProvider
	history           []any
}

func CreateSession(inferenceProvider provider.InferenceProvider) *Session {
	return &Session{
		inferenceProvider: inferenceProvider,
		history:           []any{},
	}
}

func (s *Session) RunInference(ctx context.Context, messages []models.WingmanMessage, instructions string) (*models.WingmanMessageResponse, error) {
	result, err := s.inferenceProvider.RunInference(ctx, messages, instructions)
	if err != nil {
		return nil, err
	}

	s.history = append(s.history, messages, result)

	return result, nil
}
