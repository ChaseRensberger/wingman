package models

type WingmanRole string

const (
	RoleUser      WingmanRole = "user"
	RoleAssistant WingmanRole = "assistant"
)

type WingmanContentType string

const (
	ContentTypeText       WingmanContentType = "text"
	ContentTypeToolUse    WingmanContentType = "tool_use"
	ContentTypeToolResult WingmanContentType = "tool_result"
)

type WingmanContentBlock struct {
	Type      WingmanContentType `json:"type"`
	Text      string             `json:"text,omitempty"`
	ID        string             `json:"id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Input     any                `json:"input,omitempty"`
	ToolUseID string             `json:"tool_use_id,omitempty"`
	Content   string             `json:"content,omitempty"`
	IsError   bool               `json:"is_error,omitempty"`
}

type WingmanMessage struct {
	Role    WingmanRole           `json:"role"`
	Content []WingmanContentBlock `json:"content"`
}

func NewUserMessage(text string) WingmanMessage {
	return WingmanMessage{
		Role: RoleUser,
		Content: []WingmanContentBlock{
			{Type: ContentTypeText, Text: text},
		},
	}
}

func NewAssistantMessage(text string) WingmanMessage {
	return WingmanMessage{
		Role: RoleAssistant,
		Content: []WingmanContentBlock{
			{Type: ContentTypeText, Text: text},
		},
	}
}

func NewToolResultMessage(toolUseID, content string, isError bool) WingmanMessage {
	return WingmanMessage{
		Role: RoleUser,
		Content: []WingmanContentBlock{
			{
				Type:      ContentTypeToolResult,
				ToolUseID: toolUseID,
				Content:   content,
				IsError:   isError,
			},
		},
	}
}

type WingmanToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema WingmanToolInputSchema `json:"input_schema"`
}

type WingmanToolInputSchema struct {
	Type       string                         `json:"type"`
	Properties map[string]WingmanToolProperty `json:"properties,omitempty"`
	Required   []string                       `json:"required,omitempty"`
}

type WingmanToolProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type WingmanUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type WingmanInferenceRequest struct {
	Messages     []WingmanMessage
	Tools        []WingmanToolDefinition
	MaxTokens    int
	Temperature  *float64
	Instructions string
}

type WingmanInferenceResponse struct {
	ID         string
	Content    []WingmanContentBlock
	StopReason string
	Usage      WingmanUsage
}

func (r *WingmanInferenceResponse) GetText() string {
	for _, block := range r.Content {
		if block.Type == ContentTypeText {
			return block.Text
		}
	}
	return ""
}

func (r *WingmanInferenceResponse) GetToolCalls() []WingmanContentBlock {
	var calls []WingmanContentBlock
	for _, block := range r.Content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, block)
		}
	}
	return calls
}

func (r *WingmanInferenceResponse) HasToolCalls() bool {
	return r.StopReason == "tool_use"
}
