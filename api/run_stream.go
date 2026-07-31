package api

import (
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/models"
)

// RunStreamEventType identifies one event in the one-shot POST /run stream.
type RunStreamEventType string

const (
	RunStreamEventIterationStart     RunStreamEventType = "iteration_start"
	RunStreamEventIterationEnd       RunStreamEventType = "iteration_end"
	RunStreamEventMessage            RunStreamEventType = "message"
	RunStreamEventToolProposed       RunStreamEventType = "tool_proposed"
	RunStreamEventToolAuthorized     RunStreamEventType = "tool_authorized"
	RunStreamEventToolStart          RunStreamEventType = "tool_start"
	RunStreamEventToolProgress       RunStreamEventType = "tool_progress"
	RunStreamEventToolEnd            RunStreamEventType = "tool_end"
	RunStreamEventStreamPart         RunStreamEventType = "stream_part"
	RunStreamEventCompaction         RunStreamEventType = "compaction"
	RunStreamEventContextTransformed RunStreamEventType = "context_transformed"
	RunStreamEventError              RunStreamEventType = "error"
	RunStreamEventStructuredOutput   RunStreamEventType = "structured_output"
	RunStreamEventDone               RunStreamEventType = "done"
)

// RunStreamEvent is the canonical one-shot run SSE envelope.
type RunStreamEvent struct {
	Type    RunStreamEventType `json:"type"`
	Version int                `json:"version"`
	Data    RunStreamEventData `json:"data"`
}

// RunStreamEventData is the closed union of known one-shot run payloads.
type RunStreamEventData interface {
	isRunStreamEventData()
}

// RunIterationStartEventData describes the start of one loop iteration.
type RunIterationStartEventData struct {
	Step int `json:"step"`
}

// RunIterationEndEventData describes the completion of one loop iteration.
type RunIterationEndEventData struct {
	Step int     `json:"step"`
	Turn RunTurn `json:"turn"`
}

// RunTurn is one completed model/tool loop iteration.
type RunTurn struct {
	Step              int              `json:"step"`
	ModelCallID       string           `json:"model_call_id,omitempty"`
	Attempt           int              `json:"attempt"`
	ProviderRequestID string           `json:"provider_request_id,omitempty"`
	Assistant         models.Message   `json:"assistant"`
	Results           []RunToolResult  `json:"results"`
	Usage             models.Usage     `json:"usage"`
	StartedAt         time.Time        `json:"started_at"`
	CompletedAt       time.Time        `json:"completed_at"`
	Trace             models.CallTrace `json:"trace"`
	Error             string           `json:"error,omitempty"`
}

// RunMessageEventData carries one completed message.
type RunMessageEventData struct {
	Step    int            `json:"step,omitempty"`
	Message models.Message `json:"message"`
}

// RunToolCall is one provider-proposed tool invocation.
type RunToolCall struct {
	CallID       string         `json:"call_id"`
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	MessageID    string         `json:"message_id,omitempty"`
	PartID       string         `json:"part_id,omitempty"`
	ModelCallID  string         `json:"model_call_id,omitempty"`
	Step         int            `json:"step,omitempty"`
	Ordinal      int            `json:"ordinal,omitempty"`
	ProposedAt   string         `json:"proposed_at,omitempty"`
	AuthorizedAt string         `json:"authorized_at,omitempty"`
	StartedAt    string         `json:"started_at,omitempty"`
	Name         string         `json:"name"`
	Args         map[string]any `json:"args"`
}

// RunToolProposedEventData carries one proposed tool call.
type RunToolProposedEventData struct {
	Call RunToolCall `json:"call"`
}

// RunToolAuthorizedEventData carries one authorized tool call.
type RunToolAuthorizedEventData struct {
	Call RunToolCall `json:"call"`
}

// RunToolStartEventData carries one tool call starting execution.
type RunToolStartEventData struct {
	Call RunToolCall `json:"call"`
}

// RunToolProgressEventData carries incremental tool output or metadata.
type RunToolProgressEventData struct {
	CallID      string         `json:"call_id"`
	ToolUseID   string         `json:"tool_use_id,omitempty"`
	Name        string         `json:"name"`
	OutputDelta string         `json:"output_delta,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// RunToolResult is one terminal tool execution result.
type RunToolResult struct {
	CallID     string         `json:"call_id"`
	ToolUseID  string         `json:"tool_use_id,omitempty"`
	Status     string         `json:"status,omitempty"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args"`
	Output     string         `json:"output,omitempty"`
	Structured any            `json:"structured,omitempty"`
	Error      string         `json:"error,omitempty"`
	ErrorType  string         `json:"error_type,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	IsError    bool           `json:"is_error"`
	Duration   int64          `json:"duration,omitempty"`
}

// RunToolEndEventData carries one terminal tool result.
type RunToolEndEventData struct {
	Result RunToolResult `json:"result"`
}

// RunStreamPartEventData carries one provider-neutral streaming part.
type RunStreamPartEventData struct {
	Step      int             `json:"step"`
	MessageID string          `json:"message_id,omitempty"`
	PartID    string          `json:"part_id,omitempty"`
	Revision  int64           `json:"revision,omitempty"`
	Part      json.RawMessage `json:"part"`
}

// RunContextTransformedEventData describes a history or context transformation.
type RunContextTransformedEventData struct {
	Step          int             `json:"step"`
	Phase         string          `json:"phase"`
	OriginalCount int             `json:"original_count"`
	NewCount      int             `json:"new_count"`
	Head          *models.Message `json:"head,omitempty"`
}

// RunErrorEventData is the canonical in-band run failure.
type RunErrorEventData Error

// RunStructuredOutputEventData carries validated one-shot structured output.
type RunStructuredOutputEventData struct {
	Schema  string         `json:"schema,omitempty"`
	RawJSON string         `json:"raw_json"`
	Parsed  map[string]any `json:"parsed"`
}

// RunDoneEventData summarizes a successful one-shot run.
type RunDoneEventData struct {
	Usage models.Usage `json:"usage"`
	Steps int          `json:"steps"`
}

// UnknownRunStreamEventData preserves an internal event omitted from this build.
type UnknownRunStreamEventData struct {
	Value any
}

func (RunIterationStartEventData) isRunStreamEventData()     {}
func (RunIterationEndEventData) isRunStreamEventData()       {}
func (RunMessageEventData) isRunStreamEventData()            {}
func (RunToolProposedEventData) isRunStreamEventData()       {}
func (RunToolAuthorizedEventData) isRunStreamEventData()     {}
func (RunToolStartEventData) isRunStreamEventData()          {}
func (RunToolProgressEventData) isRunStreamEventData()       {}
func (RunToolEndEventData) isRunStreamEventData()            {}
func (RunStreamPartEventData) isRunStreamEventData()         {}
func (RunContextTransformedEventData) isRunStreamEventData() {}
func (RunErrorEventData) isRunStreamEventData()              {}
func (RunStructuredOutputEventData) isRunStreamEventData()   {}
func (RunDoneEventData) isRunStreamEventData()               {}
func (UnknownRunStreamEventData) isRunStreamEventData()      {}

// MarshalJSON preserves an unknown payload without wrapping it.
func (d UnknownRunStreamEventData) MarshalJSON() ([]byte, error) {
	if d.Value == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(d.Value)
}
