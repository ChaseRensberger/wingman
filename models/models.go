// Package models defines the core types and interfaces for Wingman's
// model abstraction layer.
package models

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ------------------------------------------------------------------
// Roles
// ------------------------------------------------------------------

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ------------------------------------------------------------------
// Message
// ------------------------------------------------------------------

type Message struct {
	Role         Role           `json:"role"`
	Content      Content        `json:"content"`
	FinishReason FinishReason   `json:"finish_reason,omitempty"`
	Origin       *MessageOrigin `json:"origin,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
	Metadata     Meta           `json:"metadata,omitempty"`
}

type Meta map[string]any

type MessageOrigin struct {
	Provider string `json:"provider"`
	API      API    `json:"api"`
	ModelID  string `json:"model_id"`
}

type API string

const (
	APIOpenAIResponses   API = "openai_responses"
	APIOpenAICompletions API = "openai_completions"
	APIAnthropicMessages API = "anthropic_messages"
)

// ------------------------------------------------------------------
// Content / Part
// ------------------------------------------------------------------

type Content []Part

func (c Content) MarshalJSON() ([]byte, error) {
	raw := make([]json.RawMessage, len(c))
	for i, p := range c {
		b, err := MarshalPart(p)
		if err != nil {
			return nil, err
		}
		raw[i] = b
	}
	return json.Marshal(raw)
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = make(Content, len(raw))
	for i, b := range raw {
		p, err := UnmarshalPart(b)
		if err != nil {
			return err
		}
		(*c)[i] = p
	}
	return nil
}

// Part is the closed union of content parts.
type Part interface {
	Type() string
	isPart()
}

func (TextPart) isPart()       {}
func (ImagePart) isPart()      {}
func (ReasoningPart) isPart()  {}
func (ToolCallPart) isPart()   {}
func (ToolResultPart) isPart() {}
func (OpaquePart) isPart()     {}

// TextPart is a plain text block.
type TextPart struct {
	Text string `json:"text"`
}

func (TextPart) Type() string { return "text" }

// ImagePart is an image reference.
type ImagePart struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

func (ImagePart) Type() string { return "image" }

// ReasoningPart carries model reasoning text.
type ReasoningPart struct {
	Reasoning string `json:"reasoning"`
}

func (ReasoningPart) Type() string { return "reasoning" }

// ToolCallPart is a completed tool call inside a message.
type ToolCallPart struct {
	CallID string         `json:"call_id"`
	Name   string         `json:"name"`
	Input  map[string]any `json:"input"`
}

func (ToolCallPart) Type() string { return "tool_call" }

// ToolResultPart is the outcome of a tool execution.
type ToolResultPart struct {
	CallID   string `json:"call_id"`
	Output   []Part `json:"output"`
	IsError  bool   `json:"is_error"`
	Metadata Meta   `json:"metadata,omitempty"`
}

func (ToolResultPart) Type() string { return "tool_result" }

func (p ToolResultPart) MarshalJSON() ([]byte, error) {
	raw := make([]json.RawMessage, len(p.Output))
	for i, part := range p.Output {
		b, err := MarshalPart(part)
		if err != nil {
			return nil, err
		}
		raw[i] = b
	}
	return json.Marshal(struct {
		CallID   string            `json:"call_id"`
		Output   []json.RawMessage `json:"output"`
		IsError  bool              `json:"is_error"`
		Metadata Meta              `json:"metadata,omitempty"`
	}{p.CallID, raw, p.IsError, p.Metadata})
}

func (p *ToolResultPart) UnmarshalJSON(data []byte) error {
	var raw struct {
		CallID   string            `json:"call_id"`
		Output   []json.RawMessage `json:"output"`
		IsError  bool              `json:"is_error"`
		Metadata Meta              `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.CallID = raw.CallID
	p.IsError = raw.IsError
	p.Metadata = raw.Metadata
	p.Output = make([]Part, len(raw.Output))
	for i, b := range raw.Output {
		part, err := UnmarshalPart(b)
		if err != nil {
			return err
		}
		p.Output[i] = part
	}
	return nil
}

// OpaquePart is a catch-all carrier for plugin-defined part types.
// Raw should be the complete JSON payload, including the "type" field.
type OpaquePart struct {
	TypeName string `json:"-"`
	Raw      []byte `json:"raw"`
}

func (p OpaquePart) Type() string { return p.TypeName }

func (p OpaquePart) MarshalJSON() ([]byte, error) {
	if p.Raw != nil {
		return p.Raw, nil
	}
	return json.Marshal(struct {
		Type string `json:"type"`
		Raw  []byte `json:"raw"`
	}{p.TypeName, nil})
}

func (p *OpaquePart) UnmarshalJSON(data []byte) error {
	p.Raw = data
	var wrapper struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	p.TypeName = wrapper.Type
	return nil
}

// ------------------------------------------------------------------
// Part registry
// ------------------------------------------------------------------

type PartUnmarshaler func(data []byte) (Part, error)

var (
	partRegistryMu sync.RWMutex
	partRegistry   = map[string]PartUnmarshaler{
		"text": func(data []byte) (Part, error) {
			var p TextPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"image": func(data []byte) (Part, error) {
			var p ImagePart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"reasoning": func(data []byte) (Part, error) {
			var p ReasoningPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_call": func(data []byte) (Part, error) {
			var p ToolCallPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_result": func(data []byte) (Part, error) {
			var p ToolResultPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
	}
)

func RegisterPart(typeName string, fn PartUnmarshaler) {
	partRegistryMu.Lock()
	defer partRegistryMu.Unlock()
	partRegistry[typeName] = fn
}

func MarshalPart(p Part) ([]byte, error) {
	// OpaquePart carries its own fully-formed JSON.
	if op, ok := p.(OpaquePart); ok {
		return op.Raw, nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(b) == 2 && b[0] == '{' && b[1] == '}' {
		return fmt.Appendf(nil, `{"type":%q}`, p.Type()), nil
	}
	// Prepend type as the first field after the opening brace.
	return fmt.Appendf(nil, `{"type":%q,%s`, p.Type(), string(b[1:])), nil
}

func UnmarshalPart(data []byte) (Part, error) {
	var wrapper struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	partRegistryMu.RLock()
	fn, ok := partRegistry[wrapper.Type]
	partRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown part type: %s", wrapper.Type)
	}
	return fn(data)
}

// ------------------------------------------------------------------
// Request / Model
// ------------------------------------------------------------------

type Request struct {
	Model           ModelRef       `json:"model,omitempty"`
	System          string         `json:"system,omitempty"`
	Messages        []Message      `json:"messages"`
	Tools           []ToolDef      `json:"tools,omitempty"`
	ToolChoice      ToolChoice     `json:"tool_choice,omitempty"`
	Generation      Generation     `json:"generation,omitempty"`
	Capabilities    Capabilities   `json:"capabilities,omitempty"`
	ProviderOptions ProviderBag    `json:"provider_options,omitempty"`
	HTTP            HTTPOptions    `json:"http,omitempty"`
	ResponseFormat  ResponseFormat `json:"response_format,omitempty"`
	OutputSchema    *OutputSchema  `json:"output_schema,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
}

// ModelRef identifies one concrete provider/model route. New WingModels APIs
// use provider-qualified model refs such as "openai/gpt-5.5" instead of
// separate conceptual provider and model fields.
type ModelRef struct {
	Provider      string            `json:"provider,omitempty"`
	ID            string            `json:"id,omitempty"`
	API           API               `json:"api,omitempty"`
	BaseURL       string            `json:"base_url,omitempty"`
	Env           []string          `json:"env,omitempty"`
	ContextWindow int               `json:"context_window,omitempty"`
	MaxOutput     int               `json:"max_output,omitempty"`
	Capabilities  ModelCapabilities `json:"capabilities,omitempty"`
}

// Ref returns the provider-qualified model reference, if both parts are set.
func (m ModelRef) Ref() string {
	if m.Provider == "" || m.ID == "" {
		return ""
	}
	return m.Provider + "/" + m.ID
}

// Generation contains portable sampling/output knobs.
type Generation struct {
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// ProviderBag carries provider-specific request options keyed by provider ID.
type ProviderBag map[string]map[string]any

// HTTPOptions is a last-resort request overlay for advanced provider knobs.
type HTTPOptions struct {
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
}

// ResponseFormat describes requested output constraints.
type ResponseFormat struct {
	Type   string         `json:"type,omitempty"`
	Name   string         `json:"name,omitempty"`
	Schema map[string]any `json:"schema,omitempty"`
	Strict bool           `json:"strict,omitempty"`
}

// ------------------------------------------------------------------
// ToolDef
// ------------------------------------------------------------------

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ------------------------------------------------------------------
// ToolChoice
// ------------------------------------------------------------------

type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceRequired ToolChoice = "required"
	ToolChoiceNone     ToolChoice = "none"
)

// ------------------------------------------------------------------
// Capabilities
// ------------------------------------------------------------------

type Capabilities struct {
	Thinking bool `json:"thinking,omitempty"`
}

// ------------------------------------------------------------------
// Usage
// ------------------------------------------------------------------

type Usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	TotalTokens       int `json:"total_tokens"`
	ReasoningTokens   int `json:"reasoning_tokens,omitempty"`
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens  int `json:"cache_write_tokens,omitempty"`
}

func (u Usage) Empty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 && u.ReasoningTokens == 0 && u.CachedInputTokens == 0 && u.CacheWriteTokens == 0
}

func (u Usage) ContextTokens() int {
	computed := u.BillableInputTokens() + u.VisibleOutputTokens() + safeTokenCount(u.ReasoningTokens) + safeTokenCount(u.CachedInputTokens) + safeTokenCount(u.CacheWriteTokens)
	if computed == 0 && u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return computed
}

func (u Usage) TotalOrComputed() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.ContextTokens()
}

func (u Usage) BillableInputTokens() int {
	return safeTokenCount(u.InputTokens - u.CachedInputTokens - u.CacheWriteTokens)
}

func (u Usage) VisibleOutputTokens() int {
	return safeTokenCount(u.OutputTokens - u.ReasoningTokens)
}

func (u Usage) ContextPercent(contextWindow int) float64 {
	if contextWindow <= 0 {
		return 0
	}
	return float64(u.ContextTokens()) / float64(contextWindow) * 100
}

func safeTokenCount(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// ------------------------------------------------------------------
// FinishReason
// ------------------------------------------------------------------

type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonToolCalls FinishReason = "tool_calls"
	FinishReasonMaxTokens FinishReason = "max_tokens"
	FinishReasonAborted   FinishReason = "aborted"
	FinishReasonError     FinishReason = "error"
)

// ------------------------------------------------------------------
// ModelInfo / ModelCapabilities
// ------------------------------------------------------------------

type ModelInfo struct {
	Provider          string            `json:"provider"`
	ID                string            `json:"id"`
	API               API               `json:"api,omitempty"`
	BaseURL           string            `json:"base_url,omitempty"`
	Env               []string          `json:"env,omitempty"`
	ContextWindow     int               `json:"context_window,omitempty"`
	MaxOutput         int               `json:"max_output,omitempty"`
	Capabilities      ModelCapabilities `json:"capabilities"`
	InputCostPerMTok  float64           `json:"input_cost_per_mtok,omitempty"`
	OutputCostPerMTok float64           `json:"output_cost_per_mtok,omitempty"`
}

type ModelCapabilities struct {
	Tools            bool `json:"tools"`
	Images           bool `json:"images"`
	Reasoning        bool `json:"reasoning"`
	StructuredOutput bool `json:"structured_output"`
}

// ------------------------------------------------------------------
// OutputSchema
// ------------------------------------------------------------------

type OutputSchema struct {
	Name   string         `json:"name,omitempty"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict,omitempty"`
}

// ------------------------------------------------------------------
// ProviderOptions (unused but reserved)
// ------------------------------------------------------------------

type ProviderOptions struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func NewUserText(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: Content{TextPart{Text: text}},
	}
}

func NewAssistantText(text string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: Content{TextPart{Text: text}},
	}
}
