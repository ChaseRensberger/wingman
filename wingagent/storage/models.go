package storage

import (
	"github.com/chaserensberger/wingman/wingagent/core"
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

// Fleet and Formation types are archived; their definitions live in
// _archive/ for reference. Do not add new consumers.

type AuthCredential struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type Auth struct {
	Providers map[string]AuthCredential `json:"providers"`
	UpdatedAt string                    `json:"updated_at"`
}
