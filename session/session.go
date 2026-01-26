package session

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"

	"wingman/models"
)

type Session struct {
	ID      string
	History []models.WingmanMessage
}

func New() *Session {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	return &Session{
		ID:      id.String(),
		History: []models.WingmanMessage{},
	}
}

func (s *Session) AddMessage(msg models.WingmanMessage) {
	s.History = append(s.History, msg)
}

func (s *Session) AddMessages(msgs ...models.WingmanMessage) {
	s.History = append(s.History, msgs...)
}

func (s *Session) Messages() []models.WingmanMessage {
	return s.History
}

func (s *Session) Clear() {
	s.History = []models.WingmanMessage{}
}
