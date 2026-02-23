// Package core defines the canonical types and interfaces shared across all
// Wingman components — the SDK, the HTTP server, providers, tools, and the
// actor system. Every other package in Wingman imports from core; core itself
// has no Wingman dependencies.
package core

import "context"

// ============================================================
//  Roles and content types
// ============================================================

// Role is the speaker in a conversation turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentType identifies what a ContentBlock carries.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ============================================================
//  Message types
// ============================================================

// ContentBlock is a single piece of content within a Message. The fields that
// are populated depend on Type.
//
//   - text:        Text is set.
//   - tool_use:    ID, Name, and Input are set.
//   - tool_result: ToolUseID, Content, and optionally IsError are set.
type ContentBlock struct {
	Type      ContentType `json:"type"`
	Text      string      `json:"text,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Input     any         `json:"input,omitempty"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   string      `json:"content,omitempty"`
	IsError   bool        `json:"is_error,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// NewUserMessage creates a user turn containing plain text.
func NewUserMessage(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: []ContentBlock{{Type: ContentTypeText, Text: text}},
	}
}

// NewAssistantMessage creates an assistant turn containing plain text.
func NewAssistantMessage(text string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: []ContentBlock{{Type: ContentTypeText, Text: text}},
	}
}

// NewToolResultMessage creates a user turn carrying the result of a tool call.
func NewToolResultMessage(toolUseID, content string, isError bool) Message {
	return Message{
		Role: RoleUser,
		Content: []ContentBlock{{
			Type:      ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   content,
			IsError:   isError,
		}},
	}
}

// ============================================================
//  Tool definitions (sent to the LLM so it knows what's available)
// ============================================================

// ToolDefinition describes a tool to the language model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"input_schema"`
}

// ToolInputSchema describes the JSON Schema for a tool's input parameters.
type ToolInputSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]ToolProperty `json:"properties,omitempty"`
	Required   []string                `json:"required,omitempty"`
}

// ToolProperty describes one parameter of a tool.
type ToolProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ============================================================
//  Usage / token accounting
// ============================================================

// Usage tracks token consumption for one inference call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ============================================================
//  Inference request / response (provider contract)
// ============================================================

// InferenceRequest is the provider-agnostic input to a language model call.
// Providers translate this into their wire format before sending.
type InferenceRequest struct {
	Messages     []Message
	Tools        []ToolDefinition
	Instructions string // system prompt
	OutputSchema map[string]any
}

// InferenceResponse is the provider-agnostic output of a language model call.
type InferenceResponse struct {
	ID         string
	Content    []ContentBlock
	StopReason string
	Usage      Usage
}

// GetText returns the first text content block, or empty string.
func (r *InferenceResponse) GetText() string {
	for _, b := range r.Content {
		if b.Type == ContentTypeText {
			return b.Text
		}
	}
	return ""
}

// GetToolCalls returns all tool_use content blocks.
func (r *InferenceResponse) GetToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, b := range r.Content {
		if b.Type == ContentTypeToolUse {
			calls = append(calls, b)
		}
	}
	return calls
}

// HasToolCalls reports whether the model stopped to call one or more tools.
func (r *InferenceResponse) HasToolCalls() bool {
	return r.StopReason == "tool_use"
}

// ============================================================
//  Streaming
// ============================================================

// StreamEventType identifies what kind of streaming event is being emitted.
type StreamEventType string

const (
	EventMessageStart      StreamEventType = "message_start"
	EventContentBlockStart StreamEventType = "content_block_start"
	EventTextDelta         StreamEventType = "text_delta"
	EventInputJSONDelta    StreamEventType = "input_json_delta"
	EventContentBlockStop  StreamEventType = "content_block_stop"
	EventMessageDelta      StreamEventType = "message_delta"
	EventMessageStop       StreamEventType = "message_stop"
	EventPing              StreamEventType = "ping"
	EventError             StreamEventType = "error"
)

// StreamEvent is a single event emitted during a streaming inference call.
type StreamEvent struct {
	Type         StreamEventType     `json:"type"`
	Text         string              `json:"text,omitempty"`
	InputJSON    string              `json:"input_json,omitempty"`
	Index        int                 `json:"index,omitempty"`
	ContentBlock *StreamContentBlock `json:"content_block,omitempty"`
	StopReason   string              `json:"stop_reason,omitempty"`
	Usage        *Usage              `json:"usage,omitempty"`
	Error        error               `json:"-"` // not serialised; check Err() on the stream
}

// StreamContentBlock carries the metadata for a content_block_start event.
type StreamContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

// ============================================================
//  Provider interface
// ============================================================

// Provider is the interface that every LLM backend must implement. Wingman
// calls RunInference for blocking requests and StreamInference when the caller
// wants incremental token delivery.
type Provider interface {
	RunInference(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
	StreamInference(ctx context.Context, req InferenceRequest) (Stream, error)
}

// Stream is returned by Provider.StreamInference. Callers iterate with Next
// and read events with Event. After Next returns false the accumulated
// InferenceResponse is available via Response.
type Stream interface {
	Next() bool
	Event() StreamEvent
	Err() error
	Close() error
	Response() *InferenceResponse
}

// ============================================================
//  Tool interface
// ============================================================

// Tool is the interface every Wingman tool must implement. Built-in tools
// (bash, read, write, edit, glob, grep, webfetch) and custom user-defined
// tools both implement this interface.
type Tool interface {
	Name() string
	Description() string
	Definition() ToolDefinition
	Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
