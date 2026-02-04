package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *JSONStore) agentPath(id string) string {
	return filepath.Join(s.basePath, "agents", id+".json")
}

func (s *JSONStore) CreateAgent(agent *Agent) error {
	if agent.ID == "" {
		agent.ID = NewID()
	}
	now := Now()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	path := s.agentPath(agent.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("agent already exists: %s", agent.ID)
	}

	return s.writeJSON(path, agent)
}

func (s *JSONStore) GetAgent(id string) (*Agent, error) {
	var agent Agent
	if err := s.readJSON(s.agentPath(id), &agent); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent not found: %s", id)
		}
		return nil, err
	}
	return &agent, nil
}

func (s *JSONStore) ListAgents() ([]*Agent, error) {
	files, err := s.listDir(filepath.Join(s.basePath, "agents"))
	if err != nil {
		return nil, err
	}

	agents := make([]*Agent, 0, len(files))
	for _, file := range files {
		var agent Agent
		if err := s.readJSON(file, &agent); err != nil {
			continue
		}
		agents = append(agents, &agent)
	}
	return agents, nil
}

func (s *JSONStore) UpdateAgent(agent *Agent) error {
	path := s.agentPath(agent.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}

	agent.UpdatedAt = Now()
	return s.writeJSON(path, agent)
}

func (s *JSONStore) DeleteAgent(id string) error {
	path := s.agentPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent not found: %s", id)
	}
	return s.deleteJSON(path)
}
