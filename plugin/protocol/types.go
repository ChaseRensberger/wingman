// Package protocol defines the versioned messages shared by Wingman and plugin SDKs.
package protocol

import (
	"encoding/json"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

// Version identifies the plugin contract.
const Version = 2

// Identity identifies the client and execution that own a callback.
type Identity struct {
	ClientID  string `json:"client_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	WorkDir   string `json:"work_dir,omitempty"`
}

// Tool describes a model-callable operation.
type Tool struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	InputSchema     map[string]any         `json:"input_schema"`
	OutputSchema    map[string]any         `json:"output_schema,omitempty"`
	Sequential      bool                   `json:"sequential,omitempty"`
	DirectoryScoped bool                   `json:"directory_scoped,omitempty"`
	Permission      *tool.PermissionTarget `json:"permission,omitempty"`
}

// Action describes a plugin-owned session command.
type Action struct {
	ID          string          `json:"id"`
	Command     string          `json:"command"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// Provider describes models served by a plugin's native implementation.
type Provider struct {
	Meta   provider.ProviderMeta       `json:"meta"`
	Models map[string]models.ModelInfo `json:"models"`
}

// Method describes a plugin-defined RPC operation.
type Method struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

// Contributions declares the executable capabilities of a plugin generation.
type Contributions struct {
	Transforms      bool         `json:"transforms,omitempty"`
	Auth            []AuthMethod `json:"auth,omitempty"`
	Tools           []Tool       `json:"tools,omitempty"`
	Hooks           []string     `json:"hooks,omitempty"`
	Actions         []Action     `json:"actions,omitempty"`
	Providers       []Provider   `json:"providers,omitempty"`
	Methods         []Method     `json:"methods,omitempty"`
	Parts           []string     `json:"parts,omitempty"`
	ContextBoundary string       `json:"context_boundary,omitempty"`
}

// ActionCall supplies the session state for a plugin-owned command.
type ActionCall struct {
	Name      string           `json:"name"`
	Input     json.RawMessage  `json:"input"`
	Identity  Identity         `json:"context"`
	History   []models.Message `json:"history"`
	Model     models.ModelRef  `json:"model"`
	ModelInfo models.ModelInfo `json:"model_info"`
}

// Hook contains the serializable context of a lifecycle interception.
type Hook struct {
	Name            string                 `json:"name"`
	Identity        Identity               `json:"context"`
	System          string                 `json:"system,omitempty"`
	Messages        []models.Message       `json:"messages"`
	Model           models.ModelRef        `json:"model"`
	ModelInfo       models.ModelInfo       `json:"model_info"`
	Step            int                    `json:"step,omitempty"`
	Request         models.Request         `json:"request"`
	Usage           models.Usage           `json:"usage"`
	Tools           []models.ToolDef       `json:"tools"`
	Permissions     permission.Ruleset     `json:"permissions,omitempty"`
	Call            run.ToolCall           `json:"call"`
	Result          run.ToolResult         `json:"result"`
	Params          run.SamplingParams     `json:"params"`
	Continuations   int                    `json:"continuations,omitempty"`
	RunResult       run.Result             `json:"run_result"`
	Turn            run.Turn               `json:"turn"`
	Candidate       *models.Message        `json:"candidate,omitempty"`
	Decision        run.CompletionDecision `json:"decision"`
	Skip            string                 `json:"skip,omitempty"`
	Error           string                 `json:"error,omitempty"`
	ContextOverflow bool                   `json:"context_overflow,omitempty"`
}

// HostCall binds a plugin request to an active invocation rather than a claimed client ID.
type HostCall struct {
	InvocationID int64           `json:"invocation_id"`
	ScopeToken   string          `json:"scope_token,omitempty"`
	Method       string          `json:"method"`
	Input        json.RawMessage `json:"input"`
}

// HTTPRequest calls the normal Wingman API with the invocation's client identity.
type HTTPRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// HTTPResponse contains the result of a host API call.
type HTTPResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// ProviderCall supplies a model request and only that provider's credential.
type ProviderCall struct {
	Identity     Identity                 `json:"context"`
	Provider     string                   `json:"provider"`
	Request      models.Request           `json:"request"`
	Credential   provider.Credential      `json:"credential"`
	Options      provider.ProviderOptions `json:"options"`
	AuthDisabled bool                     `json:"auth_disabled,omitempty"`
}

// StreamUpdate carries a model stream event correlated to its active request.
type StreamUpdate struct {
	RequestID int64           `json:"request_id"`
	Part      json.RawMessage `json:"part"`
}

// Event is an observed runtime event associated with its owner.
type Event struct {
	Identity Identity        `json:"context"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
}

// Prompt is an incoming user message before instruction resolution and admission.
type Prompt struct {
	Identity  Identity `json:"context"`
	RequestID string   `json:"request_id,omitempty"`
	Message   string   `json:"message"`
}
