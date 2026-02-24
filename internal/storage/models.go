package storage

import (
	"github.com/chaserensberger/wingman/core"
)

type Agent struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Instructions string         `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Model        string         `json:"model,omitempty"`
	Options      map[string]any `json:"options,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type Session struct {
	ID        string         `json:"id"`
	WorkDir   string         `json:"work_dir,omitempty"`
	History   []core.Message `json:"history"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
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

type Formation struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Version    int            `json:"version"`
	Definition map[string]any `json:"definition"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

type AuthCredential struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type Auth struct {
	Providers map[string]AuthCredential `json:"providers"`
	UpdatedAt string                    `json:"updated_at"`
}
