package store

import "time"

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
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	WorkDir   string `json:"work_dir,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// StoredMessage is a single message row for a session.
type StoredMessage struct {
	ID           string
	SessionID    string
	Idx          int
	Role         string
	MetadataJSON []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Parts        []StoredPart
}

// StoredPart is a single content part belonging to a message.
// PayloadJSON is opaque to the store: serialization and interpretation
// belong to the agent/session layer; Kind is a free-form discriminator
// string.
type StoredPart struct {
	ID          string
	MessageID   string
	Sequence    int
	Kind        string
	PayloadJSON []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
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
