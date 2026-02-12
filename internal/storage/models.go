package storage

import (
	"wingman/models"
)

type ProviderConfig struct {
	ID          string   `json:"id"`
	Model       string   `json:"model,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type Agent struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Instructions string          `json:"instructions,omitempty"`
	Tools        []string        `json:"tools,omitempty"`
	Provider     *ProviderConfig `json:"provider,omitempty"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

type Session struct {
	ID        string                  `json:"id"`
	WorkDir   string                  `json:"work_dir,omitempty"`
	History   []models.WingmanMessage `json:"history"`
	CreatedAt string                  `json:"created_at"`
	UpdatedAt string                  `json:"updated_at"`
}

type FleetStatus string

const (
	FleetStatusStopped FleetStatus = "stopped"
	FleetStatusRunning FleetStatus = "running"
)

type Fleet struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	AgentID     string      `json:"agent_id"`
	WorkerCount int         `json:"worker_count"`
	WorkDir     string      `json:"work_dir,omitempty"`
	Status      FleetStatus `json:"status"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
}

type FormationStatus string

const (
	FormationStatusStopped FormationStatus = "stopped"
	FormationStatusRunning FormationStatus = "running"
)

type FormationRole struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Count   int    `json:"count"`
}

type FormationEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

type Formation struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	WorkDir   string          `json:"work_dir,omitempty"`
	Roles     []FormationRole `json:"roles"`
	Edges     []FormationEdge `json:"edges"`
	Status    FormationStatus `json:"status"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type AuthCredential struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type Auth struct {
	Providers map[string]AuthCredential `json:"providers"`
	UpdatedAt string                    `json:"updated_at"`
}
