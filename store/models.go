package store

import (
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/skill"
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
	ID               string `json:"id"`
	Title            string `json:"title,omitempty"`
	WorkDir          string `json:"work_dir,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	AggregateVersion int64  `json:"version"`
}

// InstructionSource identifies one file included in effective run instructions.
type InstructionSource struct {
	Kind       string    `json:"kind"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	ResolvedAt time.Time `json:"resolved_at"`
	Order      int       `json:"order"`
}

const (
	SessionRunStatusQueued    = "queued"
	SessionRunStatusRunning   = "running"
	SessionRunStatusCompleted = "completed"
	SessionRunStatusFailed    = "failed"
	SessionRunStatusAborted   = "aborted"
)

const (
	ToolUseStatusProposed    = "proposed"
	ToolUseStatusAuthorized  = "authorized"
	ToolUseStatusStarted     = "started"
	ToolUseStatusCompleted   = "completed"
	ToolUseStatusFailed      = "failed"
	ToolUseStatusInterrupted = "interrupted"
	ToolUseStatusDeclined    = "declined"
)

const (
	PermissionRequestStatusPending     = "pending"
	PermissionRequestStatusApproved    = "approved"
	PermissionRequestStatusRejected    = "rejected"
	PermissionRequestStatusTimedOut    = "timed_out"
	PermissionRequestStatusInterrupted = "interrupted"
)

const (
	PermissionResponseOnce   = "once"
	PermissionResponseAlways = "always"
	PermissionResponseReject = "reject"
)

// PermissionRequest records one interactive authorization decision.
type PermissionRequest struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	RunID        string    `json:"run_id,omitempty"`
	ToolUseID    string    `json:"tool_use_id,omitempty"`
	CallID       string    `json:"call_id,omitempty"`
	Action       string    `json:"action"`
	Resources    []string  `json:"resources"`
	Status       string    `json:"status"`
	Response     string    `json:"response,omitempty"`
	ErrorType    string    `json:"error_type,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ResolvedAt   time.Time `json:"resolved_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PermissionGrant is a session-scoped authorization remembered by an
// approved "always" response.
type PermissionGrant struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	CreatedAt time.Time `json:"created_at"`
}

// PermissionRequestTransition is an atomic request state change and event.
type PermissionRequestTransition struct {
	Request PermissionRequest
	Event   SessionEvent
	Changed bool
}

// PermissionRequestResolution describes an expected-pending terminal update.
type PermissionRequestResolution struct {
	SessionID      string
	RequestID      string
	ExpectedStatus string
	Status         string
	Response       string
	ErrorType      string
	ErrorMessage   string
}

// ToolUse records one durable tool invocation lifecycle.
type ToolUse struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	RunID              string    `json:"run_id,omitempty"`
	ModelCallID        string    `json:"model_call_id,omitempty"`
	AssistantMessageID string    `json:"assistant_message_id,omitempty"`
	PartID             string    `json:"part_id,omitempty"`
	Step               int       `json:"step"`
	Ordinal            int       `json:"ordinal"`
	CallID             string    `json:"call_id,omitempty"`
	Name               string    `json:"name"`
	Status             string    `json:"status"`
	InputJSON          []byte    `json:"-"`
	Output             string    `json:"output,omitempty"`
	StructuredJSON     []byte    `json:"-"`
	MetadataJSON       []byte    `json:"-"`
	ErrorType          string    `json:"error_type,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	ProposedAt         time.Time `json:"proposed_at,omitempty"`
	AuthorizedAt       time.Time `json:"authorized_at,omitempty"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// MarshalJSON exposes stored JSON payloads as JSON values rather than base64.
func (u ToolUse) MarshalJSON() ([]byte, error) {
	type alias ToolUse
	return json.Marshal(struct {
		*alias
		Input      json.RawMessage `json:"input,omitempty"`
		Structured json.RawMessage `json:"structured,omitempty"`
		Metadata   json.RawMessage `json:"metadata,omitempty"`
	}{
		alias:      (*alias)(&u),
		Input:      u.InputJSON,
		Structured: u.StructuredJSON,
		Metadata:   u.MetadataJSON,
	})
}

// SessionRun is a durably admitted prompt and its immutable effective agent
// configuration. Runs are claimed in sequence order per session.
type SessionRun struct {
	ID                    string              `json:"id"`
	SessionID             string              `json:"session_id"`
	RequestID             string              `json:"request_id,omitempty"`
	RequestHash           string              `json:"-"`
	AdmittedVersion       int64               `json:"admitted_version"`
	WorkDir               string              `json:"work_dir,omitempty"`
	WorkspaceID           string              `json:"workspace_id,omitempty"`
	ClientID              string              `json:"client_id,omitempty"`
	Sequence              int                 `json:"sequence"`
	Status                string              `json:"status"`
	Message               string              `json:"message"`
	Agent                 Agent               `json:"agent"`
	EffectiveInstructions string              `json:"effective_instructions"`
	InstructionSources    []InstructionSource `json:"instruction_sources,omitempty"`
	Skills                []skill.Skill       `json:"skills,omitempty"`
	OutputSchemaJSON      []byte              `json:"-"`
	ErrorType             string              `json:"error_type,omitempty"`
	ErrorMessage          string              `json:"error_message,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	StartedAt             time.Time           `json:"started_at,omitempty"`
	CompletedAt           time.Time           `json:"completed_at,omitempty"`
	UpdatedAt             time.Time           `json:"updated_at"`
}

// SessionRunAdmission is the result of admitting a run to a session queue.
type SessionRunAdmission struct {
	Run            SessionRun
	SessionVersion int64
	Created        bool
	QueuedEvent    SessionEvent
}

// SessionRunTransition is an atomic run state transition and its durable event.
type SessionRunTransition struct {
	Run     SessionRun
	Event   SessionEvent
	Changed bool
}

// SessionRunSettlement authoritatively completes, fails, or aborts a run.
type SessionRunSettlement struct {
	ID             string
	ExpectedStatus string
	Status         string
	ErrorType      string
	ErrorMessage   string
	EventData      map[string]any
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
	RunID        string
	Idx          int
	Role         string
	Revision     int64
	State        string
	MetadataJSON []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Parts        []StoredPart
}

// SessionEvent is one server-sent event for a session. Durable events have
// a session-scoped Seq and are stored for replay. Live-only events use the
// same wire shape but are not persisted.
type SessionEvent struct {
	ID            string          `json:"id"`
	SchemaVersion int             `json:"schema_version"`
	Type          string          `json:"type"`
	Time          time.Time       `json:"-"`
	SessionID     string          `json:"session_id,omitempty"`
	Seq           int64           `json:"seq,omitempty"`
	DataJSON      []byte          `json:"-"`
	Data          json.RawMessage `json:"data"`
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
		ID            string              `json:"id"`
		SchemaVersion int                 `json:"schema_version"`
		Type          string              `json:"type"`
		Time          string              `json:"time,omitempty"`
		Cursor        *sessionEventCursor `json:"cursor,omitempty"`
		Data          json.RawMessage     `json:"data"`
	}{
		ID:            e.ID,
		SchemaVersion: e.SchemaVersion,
		Type:          e.Type,
		Time:          timeValue,
		Cursor:        cursor,
		Data:          data,
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
	ID                   string          `json:"id"`
	SessionID            string          `json:"session_id"`
	RunID                string          `json:"run_id,omitempty"`
	AssistantMessageID   string          `json:"assistant_message_id,omitempty"`
	Step                 int             `json:"step"`
	Attempt              int             `json:"attempt"`
	Status               string          `json:"status"`
	AgentID              string          `json:"agent_id,omitempty"`
	ModelRef             string          `json:"model_ref,omitempty"`
	Provider             string          `json:"provider,omitempty"`
	ProviderRequestID    string          `json:"provider_request_id,omitempty"`
	API                  string          `json:"api,omitempty"`
	ModelID              string          `json:"model_id,omitempty"`
	FinishReason         string          `json:"finish_reason,omitempty"`
	StopReason           string          `json:"stop_reason,omitempty"`
	ErrorType            string          `json:"error_type,omitempty"`
	ErrorMessage         string          `json:"error_message,omitempty"`
	InputTokens          int             `json:"input_tokens"`
	OutputTokens         int             `json:"output_tokens"`
	ReasoningTokens      int             `json:"reasoning_tokens,omitempty"`
	CachedInputTokens    int             `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens     int             `json:"cache_write_tokens,omitempty"`
	TotalTokens          int             `json:"total_tokens"`
	ContextTokens        int             `json:"context_tokens"`
	ContextWindow        int             `json:"context_window,omitempty"`
	ContextPercent       float64         `json:"context_percent,omitempty"`
	Cost                 *float64        `json:"cost,omitempty"`
	StructuredOutputJSON []byte          `json:"-"`
	MetadataJSON         []byte          `json:"-"`
	Trace                json.RawMessage `json:"trace,omitempty"`
	StartedAt            time.Time       `json:"started_at"`
	CompletedAt          time.Time       `json:"completed_at,omitempty"`
	CreatedAt            time.Time       `json:"-"`
	UpdatedAt            time.Time       `json:"-"`
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
	DefaultClientID   = "cli_wingclient"
	DefaultClientName = "WingClient"
)

// Fleet and Formation types are archived; their definitions live in
// _archive/ for reference. Do not add new consumers.

type AuthCredential struct {
	Type      string `json:"type"`
	Key       string `json:"key,omitempty"`
	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

type Auth struct {
	Providers map[string]AuthCredential `json:"providers"`
	UpdatedAt string                    `json:"updated_at"`
}
