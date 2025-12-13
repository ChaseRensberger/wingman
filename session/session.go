package session

import (
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

func (s *Session) AddToHistory(messages []models.WingmanMessage, result *models.WingmanMessageResponse) {
	s.history = append(s.history, messages, result)
}
