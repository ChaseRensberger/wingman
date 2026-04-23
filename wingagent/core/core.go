package core

import "context"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

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

type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

func NewUserMessage(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: []ContentBlock{{Type: ContentTypeText, Text: text}},
	}
}

func NewAssistantMessage(text string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: []ContentBlock{{Type: ContentTypeText, Text: text}},
	}
}

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

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"input_schema"`
}

type ToolInputSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]ToolProperty `json:"properties,omitempty"`
	Required   []string                `json:"required,omitempty"`
}

type ToolProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type InferenceRequest struct {
	Messages     []Message
	Tools        []ToolDefinition
	Instructions string
	OutputSchema map[string]any
}

type InferenceResponse struct {
	ID         string
	Content    []ContentBlock
	StopReason string
	Usage      Usage
}

func (r *InferenceResponse) GetText() string {
	for _, b := range r.Content {
		if b.Type == ContentTypeText {
			return b.Text
		}
	}
	return ""
}

func (r *InferenceResponse) GetToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, b := range r.Content {
		if b.Type == ContentTypeToolUse {
			calls = append(calls, b)
		}
	}
	return calls
}

func (r *InferenceResponse) HasToolCalls() bool {
	return r.StopReason == "tool_use"
}

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

type StreamEvent struct {
	Type         StreamEventType     `json:"type"`
	Text         string              `json:"text,omitempty"`
	InputJSON    string              `json:"input_json,omitempty"`
	Index        int                 `json:"index,omitempty"`
	ContentBlock *StreamContentBlock `json:"content_block,omitempty"`
	StopReason   string              `json:"stop_reason,omitempty"`
	Usage        *Usage              `json:"usage,omitempty"`
	Error        error               `json:"-"`
}

type StreamContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type Provider interface {
	RunInference(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
	StreamInference(ctx context.Context, req InferenceRequest) (Stream, error)
}

type Stream interface {
	Next() bool
	Event() StreamEvent
	Err() error
	Close() error
	Response() *InferenceResponse
}

type Tool interface {
	Name() string
	Description() string
	Definition() ToolDefinition
	Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
