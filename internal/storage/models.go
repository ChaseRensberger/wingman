package storage

import (
	"github.com/chaserensberger/wingman/core"
)

// Agent is the persisted representation of an agent in SQLite. Provider and
// Model are separate string fields (not a combined "provider/model" string).
type Agent struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Instructions string         `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	Provider     string         `json:"provider,omitempty"` // e.g. "anthropic"
	Model        string         `json:"model,omitempty"`    // e.g. "claude-opus-4-6"
	Options      map[string]any `json:"options,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

// Session is the persisted representation of a conversation in SQLite.
type Session struct {
	ID        string         `json:"id"`
	WorkDir   string         `json:"work_dir,omitempty"`
	History   []core.Message `json:"history"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// FleetStatus is the lifecycle state of a fleet.
type FleetStatus string

const (
	FleetStatusStopped FleetStatus = "stopped"
	FleetStatusRunning FleetStatus = "running"
)

// Fleet is the persisted representation of a fleet in SQLite.
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

// FormationStatus is the lifecycle state of a formation.
type FormationStatus string

const (
	FormationStatusStopped FormationStatus = "stopped"
	FormationStatusRunning FormationStatus = "running"
)

// FormationRole is one slot in a formation's DAG.
type FormationRole struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Count   int    `json:"count"`
}

// FormationEdge is a directed connection between two roles in a formation.
type FormationEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

// Formation is the persisted representation of a formation in SQLite.
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

// AuthCredential holds the auth credential for one provider.
type AuthCredential struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

// Auth holds all provider credentials.
type Auth struct {
	Providers map[string]AuthCredential `json:"providers"`
	UpdatedAt string                    `json:"updated_at"`
}
