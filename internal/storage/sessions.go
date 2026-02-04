package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"wingman/models"
)

func (s *JSONStore) sessionPath(id string) string {
	return filepath.Join(s.basePath, "sessions", id+".json")
}

func (s *JSONStore) CreateSession(session *Session) error {
	if session.ID == "" {
		session.ID = NewID()
	}
	now := Now()
	session.CreatedAt = now
	session.UpdatedAt = now

	if session.History == nil {
		session.History = []models.WingmanMessage{}
	}

	path := s.sessionPath(session.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("session already exists: %s", session.ID)
	}

	return s.writeJSON(path, session)
}

func (s *JSONStore) GetSession(id string) (*Session, error) {
	var session Session
	if err := s.readJSON(s.sessionPath(id), &session); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, err
	}
	return &session, nil
}

func (s *JSONStore) ListSessions() ([]*Session, error) {
	files, err := s.listDir(filepath.Join(s.basePath, "sessions"))
	if err != nil {
		return nil, err
	}

	sessions := make([]*Session, 0, len(files))
	for _, file := range files {
		var session Session
		if err := s.readJSON(file, &session); err != nil {
			continue
		}
		sessions = append(sessions, &session)
	}
	return sessions, nil
}

func (s *JSONStore) UpdateSession(session *Session) error {
	path := s.sessionPath(session.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	session.UpdatedAt = Now()
	return s.writeJSON(path, session)
}

func (s *JSONStore) DeleteSession(id string) error {
	path := s.sessionPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", id)
	}
	return s.deleteJSON(path)
}
