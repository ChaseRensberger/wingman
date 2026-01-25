package session

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"

	"wingman/models"
	"wingman/provider"
)

type Session struct {
	ID                string
	inferenceProvider provider.InferenceProvider
	history           []any
}

func CreateSession(inferenceProvider provider.InferenceProvider) *Session {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	return &Session{
		ID:                id.String(),
		inferenceProvider: inferenceProvider,
		history:           []any{},
	}
}

func (s *Session) AddToSession(messages []models.WingmanMessage, result *models.WingmanMessageResponse) {
	s.history = append(s.history, messages, result)
}
