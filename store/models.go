package store

import (
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/permission"
)

type Agent struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Instructions string             `json:"instructions,omitempty"`
	Tools        []string           `json:"tools,omitempty"`
	Permissions  permission.Ruleset `json:"permissions,omitempty"`
	ModelRef     string             `json:"model_ref,omitempty"`
	Options      map[string]any     `json:"options,omitempty"`
	OutputSchema map[string]any     `json:"output_schema,omitempty"`
	CreatedAt    string             `json:"created_at"`
	UpdatedAt    string             `json:"updated_at"`
}

type Session struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	WorkDir     string `json:"work_dir,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
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

// SessionEvent is one server-sent event for a session. Durable events have
// a session-scoped Seq and are stored for replay. Live-only events use the
// same wire shape but are not persisted.
type SessionEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Time      time.Time       `json:"-"`
	SessionID string          `json:"session_id,omitempty"`
	Seq       int64           `json:"seq,omitempty"`
	DataJSON  []byte          `json:"-"`
	Data      json.RawMessage `json:"data"`
}

type sessionEventCursor struct {
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

func (e SessionEvent) MarshalJSON() ([]byte, error) {
	data := e.Data
	if len(data) == 0 {
		data = e.DataJSON
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	timeValue := ""
	if !e.Time.IsZero() {
		timeValue = e.Time.UTC().Format(time.RFC3339Nano)
	}
	var cursor *sessionEventCursor
	if e.Seq > 0 && e.SessionID != "" {
		cursor = &sessionEventCursor{SessionID: e.SessionID, Seq: e.Seq}
	}
	return json.Marshal(struct {
		ID     string              `json:"id"`
		Type   string              `json:"type"`
		Time   string              `json:"time,omitempty"`
		Cursor *sessionEventCursor `json:"cursor,omitempty"`
		Data   json.RawMessage     `json:"data"`
	}{
		ID:     e.ID,
		Type:   e.Type,
		Time:   timeValue,
		Cursor: cursor,
		Data:   data,
	})
}

const (
	ModelCallStatusStarted   = "started"
	ModelCallStatusCompleted = "completed"
	ModelCallStatusFailed    = "failed"
	ModelCallStatusAborted   = "aborted"
)

// ModelCall records one upstream model request/response attempt. It is
// the durable source of model provenance, finish state, token usage, and
// context-window fullness for assistant turns.
type ModelCall struct {
	ID                   string    `json:"id"`
	SessionID            string    `json:"session_id"`
	AssistantMessageID   string    `json:"assistant_message_id,omitempty"`
	Step                 int       `json:"step"`
	Attempt              int       `json:"attempt"`
	Status               string    `json:"status"`
	AgentID              string    `json:"agent_id,omitempty"`
	ModelRef             string    `json:"model_ref,omitempty"`
	Provider             string    `json:"provider,omitempty"`
	API                  string    `json:"api,omitempty"`
	ModelID              string    `json:"model_id,omitempty"`
	FinishReason         string    `json:"finish_reason,omitempty"`
	StopReason           string    `json:"stop_reason,omitempty"`
	ErrorType            string    `json:"error_type,omitempty"`
	ErrorMessage         string    `json:"error_message,omitempty"`
	InputTokens          int       `json:"input_tokens"`
	OutputTokens         int       `json:"output_tokens"`
	ReasoningTokens      int       `json:"reasoning_tokens,omitempty"`
	CachedInputTokens    int       `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens     int       `json:"cache_write_tokens,omitempty"`
	TotalTokens          int       `json:"total_tokens"`
	ContextTokens        int       `json:"context_tokens"`
	ContextWindow        int       `json:"context_window,omitempty"`
	ContextPercent       float64   `json:"context_percent,omitempty"`
	Cost                 float64   `json:"cost,omitempty"`
	StructuredOutputJSON []byte    `json:"-"`
	MetadataJSON         []byte    `json:"-"`
	StartedAt            time.Time `json:"-"`
	CompletedAt          time.Time `json:"-"`
	CreatedAt            time.Time `json:"-"`
	UpdatedAt            time.Time `json:"-"`
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

// Client is a Wingman API consumer identity, such as a web UI, CLI,
// editor plugin, Formation runner, or third-party integration.
type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

const (
	DefaultClientID   = "cli_wingman"
	DefaultClientName = "Wingman"
)

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
