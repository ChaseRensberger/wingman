package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type Store interface {
	CreateAgent(agent *Agent) error
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	UpdateAgent(agent *Agent) error
	DeleteAgent(id string) error

	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(id string) error

	CreateFleet(fleet *Fleet) error
	GetFleet(id string) (*Fleet, error)
	ListFleets() ([]*Fleet, error)
	UpdateFleet(fleet *Fleet) error
	DeleteFleet(id string) error

	CreateFormation(formation *Formation) error
	GetFormation(id string) (*Formation, error)
	ListFormations() ([]*Formation, error)
	UpdateFormation(formation *Formation) error
	DeleteFormation(id string) error

	GetAuth() (*Auth, error)
	SetAuth(auth *Auth) error
}

type JSONStore struct {
	basePath string
	mu       sync.RWMutex
}

func NewJSONStore(basePath string) (*JSONStore, error) {
	dirs := []string{
		basePath,
		filepath.Join(basePath, "agents"),
		filepath.Join(basePath, "sessions"),
		filepath.Join(basePath, "fleets"),
		filepath.Join(basePath, "formations"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return &JSONStore{basePath: basePath}, nil
}

func DefaultBasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "wingman"), nil
}

func NewID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

func (s *JSONStore) readJSON(path string, v any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *JSONStore) writeJSON(path string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *JSONStore) deleteJSON(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(path)
}

func (s *JSONStore) listDir(dir string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}
