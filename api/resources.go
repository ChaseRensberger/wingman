package api

import (
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
)

// Agent is one persisted agent definition.
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

// CreateAgentRequest creates an agent definition.
type CreateAgentRequest struct {
	Name         string             `json:"name"`
	Instructions string             `json:"instructions,omitempty"`
	Tools        []string           `json:"tools,omitempty"`
	Permissions  permission.Ruleset `json:"permissions,omitempty"`
	ModelRef     string             `json:"model_ref,omitempty"`
	ModelRoute   *models.ModelInfo  `json:"model_route,omitempty"`
	Options      map[string]any     `json:"options,omitempty"`
	OutputSchema map[string]any     `json:"output_schema,omitempty"`
}

// UpdateAgentRequest updates fields present in an agent definition.
type UpdateAgentRequest struct {
	Name         *string            `json:"name,omitempty"`
	Instructions *string            `json:"instructions,omitempty"`
	Tools        []string           `json:"tools,omitempty"`
	Permissions  permission.Ruleset `json:"permissions,omitempty"`
	ModelRef     *string            `json:"model_ref,omitempty"`
	ModelRoute   *models.ModelInfo  `json:"model_route,omitempty"`
	Options      map[string]any     `json:"options,omitempty"`
	OutputSchema map[string]any     `json:"output_schema,omitempty"`
}

// Client identifies an application consuming the Wingman API.
type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateClientRequest registers an API client identity.
type CreateClientRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateClientResponse returns a registered client.
type CreateClientResponse struct {
	Client Client `json:"client"`
}

// Workspace is one client-owned saved context.
type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ClientID  string `json:"client_id,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateWorkspaceRequest creates a saved context.
type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// UpdateWorkspaceRequest updates fields present in a saved context.
type UpdateWorkspaceRequest struct {
	Name *string `json:"name,omitempty"`
	Path *string `json:"path,omitempty"`
}

// Session is the summary returned by list, create, and metadata commands.
type Session struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	WorkDir     string `json:"work_dir,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Version     int64  `json:"version"`
}

// SessionDetail adds transcript and latest model-call state to a session.
type SessionDetail struct {
	Session
	History         []models.Message `json:"history"`
	LatestModelCall *ModelCall       `json:"latest_model_call,omitempty"`
}

// Macro identifies one project macro available to a Session.
type Macro struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	ModelRef    string `json:"model_ref,omitempty"`
}

// CreateSessionRequest creates a persistent session.
type CreateSessionRequest struct {
	Title            string `json:"title,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
}

// RenameSessionRequest changes session metadata at an expected version.
type RenameSessionRequest struct {
	Title           string `json:"title"`
	ExpectedVersion int64  `json:"expected_version"`
}

// MoveSessionRequest changes session placement at an expected version.
type MoveSessionRequest struct {
	WorkingDirectory *string `json:"working_directory,omitempty"`
	WorkspaceID      *string `json:"workspace_id,omitempty"`
	ExpectedVersion  int64   `json:"expected_version"`
}

// ModelCall describes one physical upstream model request.
type ModelCall struct {
	ID                 string          `json:"id"`
	SessionID          string          `json:"session_id"`
	RunID              string          `json:"run_id,omitempty"`
	AssistantMessageID string          `json:"assistant_message_id,omitempty"`
	Step               int             `json:"step"`
	Attempt            int             `json:"attempt"`
	Status             string          `json:"status"`
	AgentID            string          `json:"agent_id,omitempty"`
	ModelRef           string          `json:"model_ref,omitempty"`
	Provider           string          `json:"provider,omitempty"`
	ProviderRequestID  string          `json:"provider_request_id,omitempty"`
	API                string          `json:"api,omitempty"`
	ModelID            string          `json:"model_id,omitempty"`
	FinishReason       string          `json:"finish_reason,omitempty"`
	StopReason         string          `json:"stop_reason,omitempty"`
	ErrorType          string          `json:"error_type,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	InputTokens        int             `json:"input_tokens"`
	OutputTokens       int             `json:"output_tokens"`
	ReasoningTokens    int             `json:"reasoning_tokens,omitempty"`
	CachedInputTokens  int             `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens   int             `json:"cache_write_tokens,omitempty"`
	TotalTokens        int             `json:"total_tokens"`
	ContextTokens      int             `json:"context_tokens"`
	ContextWindow      int             `json:"context_window,omitempty"`
	ContextPercent     float64         `json:"context_percent,omitempty"`
	Cost               *float64        `json:"cost,omitempty"`
	Trace              json.RawMessage `json:"trace,omitempty"`
	StartedAt          time.Time       `json:"started_at"`
	CompletedAt        time.Time       `json:"completed_at,omitempty"`
}

// StatusResponse reports a completed command without a resource body.
type StatusResponse struct {
	Status string `json:"status"`
}

// ReadinessDiagnostic explains why a daemon is not ready and how to recover it.
type ReadinessDiagnostic struct {
	Subsystem      string `json:"subsystem"`
	RecoveryAction string `json:"recovery_action"`
}

// ReadinessResponse identifies a daemon that completed startup recovery.
type ReadinessResponse struct {
	Ready      bool                 `json:"ready"`
	InstanceID string               `json:"instance_id"`
	Version    string               `json:"version"`
	Diagnostic *ReadinessDiagnostic `json:"diagnostic,omitempty"`
}

// DiagnosticsResponse reports bounded daemon operational state.
type DiagnosticsResponse struct {
	QueuedRuns           int   `json:"queued_runs"`
	ActiveRuns           int   `json:"active_runs"`
	ActiveScopes         int   `json:"active_scopes"`
	EventSubscribers     int   `json:"event_subscribers"`
	SubscriberOverflows  int64 `json:"subscriber_overflows"`
	SubscriberClosures   int64 `json:"subscriber_closures"`
	SubscriberBacklog    int   `json:"subscriber_backlog"`
	SubscriberMaxBacklog int   `json:"subscriber_max_backlog"`
	PluginsRunning       int   `json:"plugins_running"`
	PluginsDegraded      int   `json:"plugins_degraded"`
	PluginsFailed        int   `json:"plugins_failed"`
	PluginLoadErrors     int   `json:"plugin_load_errors"`
}
