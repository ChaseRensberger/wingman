package api

import (
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
)

// OutputSchema names and defines structured output requested for a run.
type OutputSchema struct {
	Name   string         `json:"name,omitempty"`
	Schema map[string]any `json:"schema"`
}

// MessageSessionRequest admits one message to a persistent session.
type MessageSessionRequest struct {
	RequestID    string            `json:"request_id,omitempty"`
	AgentID      string            `json:"agent_id"`
	ModelRef     string            `json:"model_ref,omitempty"`
	ModelRoute   *models.ModelInfo `json:"model_route,omitempty"`
	Message      string            `json:"message"`
	OutputSchema *OutputSchema     `json:"output_schema,omitempty"`
}

// MessageSessionResponse identifies an admitted persistent run.
type MessageSessionResponse struct {
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	SessionVersion int64  `json:"session_version"`
}

// RunRequest executes one ephemeral turn. Agent is required in ephemeral
// mode; normal mode also accepts AgentID.
type RunRequest struct {
	AgentID          string            `json:"agent_id,omitempty"`
	Agent            *AgentSpec        `json:"agent,omitempty"`
	ModelRef         string            `json:"model_ref,omitempty"`
	ModelRoute       *models.ModelInfo `json:"model_route,omitempty"`
	Message          string            `json:"message"`
	OutputSchema     *OutputSchema     `json:"output_schema,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
}

// AgentSpec is an inline, non-persisted agent configuration for POST /run.
type AgentSpec struct {
	ID           string             `json:"id,omitempty"`
	Name         string             `json:"name"`
	Instructions string             `json:"instructions,omitempty"`
	Tools        []string           `json:"tools,omitempty"`
	Permissions  permission.Ruleset `json:"permissions,omitempty"`
	ModelRef     string             `json:"model_ref,omitempty"`
	Options      map[string]any     `json:"options,omitempty"`
	OutputSchema map[string]any     `json:"output_schema,omitempty"`
}

// AbortSessionResponse reports how many active runs were cancelled.
type AbortSessionResponse struct {
	SessionID string `json:"session_id"`
	Aborted   int    `json:"aborted"`
}

// SessionRun is one durably admitted session input and its execution state.
type SessionRun struct {
	ID              string    `json:"id"`
	SessionID       string    `json:"session_id"`
	RequestID       string    `json:"request_id,omitempty"`
	AdmittedVersion int64     `json:"admitted_version"`
	WorkDir         string    `json:"work_dir,omitempty"`
	WorkspaceID     string    `json:"workspace_id,omitempty"`
	ClientID        string    `json:"client_id,omitempty"`
	Sequence        int       `json:"sequence"`
	Status          string    `json:"status"`
	Message         string    `json:"message"`
	Agent           Agent     `json:"agent"`
	ErrorType       string    `json:"error_type,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ToolUse is one durable tool invocation lifecycle.
type ToolUse struct {
	ID                 string          `json:"id"`
	SessionID          string          `json:"session_id"`
	RunID              string          `json:"run_id,omitempty"`
	ModelCallID        string          `json:"model_call_id,omitempty"`
	AssistantMessageID string          `json:"assistant_message_id,omitempty"`
	PartID             string          `json:"part_id,omitempty"`
	Step               int             `json:"step"`
	Ordinal            int             `json:"ordinal"`
	CallID             string          `json:"call_id,omitempty"`
	Name               string          `json:"name"`
	Status             string          `json:"status"`
	Input              json.RawMessage `json:"input,omitempty"`
	Output             string          `json:"output,omitempty"`
	Structured         json.RawMessage `json:"structured,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	ErrorType          string          `json:"error_type,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	ProposedAt         time.Time       `json:"proposed_at,omitempty"`
	AuthorizedAt       time.Time       `json:"authorized_at,omitempty"`
	StartedAt          time.Time       `json:"started_at,omitempty"`
	CompletedAt        time.Time       `json:"completed_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at,omitempty"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
}

// PermissionRequest is one interactive authorization decision.
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

// PermissionGrant is a session-scoped remembered authorization.
type PermissionGrant struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	CreatedAt time.Time `json:"created_at"`
}

// PermissionReplyRequest resolves a pending permission request.
type PermissionReplyRequest struct {
	Response string `json:"response"`
}
