// Package models defines the core types and interfaces for Wingman's
// model abstraction layer.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	ID           string         `json:"id,omitempty"`
	Revision     int64          `json:"revision,omitempty"`
	State        MessageState   `json:"state,omitempty"`
	Role         Role           `json:"role"`
	Content      Content        `json:"content"`
	FinishReason FinishReason   `json:"finish_reason,omitempty"`
	Origin       *MessageOrigin `json:"origin,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
	Metadata     Meta           `json:"metadata,omitempty"`
}

type MessageState string

const (
	MessageStateInProgress MessageState = "in_progress"
	MessageStateCompleted  MessageState = "completed"
	MessageStateFailed     MessageState = "failed"
)

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
	APIOpenAICompatible  API = "openai_compatible_chat"
	APIAnthropicMessages API = "anthropic_messages"
	APIGeminiGenerate    API = "gemini_generate"
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
func (ToolPart) isPart()       {}
func (ToolCallPart) isPart()   {}
func (ToolResultPart) isPart() {}
func (OpaquePart) isPart()     {}

// TextPart is a plain text block.
type TextPart struct {
	ID               string `json:"id,omitempty"`
	Text             string `json:"text"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

func (TextPart) Type() string { return "text" }

// ImagePart is an image reference.
type ImagePart struct {
	ID               string `json:"id,omitempty"`
	URL              string `json:"url,omitempty"`
	Base64           string `json:"base64,omitempty"`
	MediaType        string `json:"media_type,omitempty"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

func (ImagePart) Type() string { return "image" }

// ReasoningPart carries model reasoning text.
type ReasoningPart struct {
	ID               string `json:"id,omitempty"`
	Reasoning        string `json:"reasoning"`
	Encrypted        string `json:"encrypted,omitempty"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

func (ReasoningPart) Type() string { return "reasoning" }

// ToolPart is an assistant-owned tool invocation and its current execution state.
// Session history stores this part; providers receive a derived tool-result message.
type ToolPart struct {
	ID               string         `json:"id,omitempty"`
	ToolUseID        string         `json:"tool_use_id,omitempty"`
	CallID           string         `json:"call_id"`
	Name             string         `json:"name"`
	State            ToolState      `json:"state"`
	Input            map[string]any `json:"input"`
	InputRaw         string         `json:"input_raw,omitempty"`
	Output           string         `json:"output,omitempty"`
	Structured       any            `json:"structured,omitempty"`
	Metadata         Meta           `json:"metadata,omitempty"`
	ProviderExecuted bool           `json:"provider_executed,omitempty"`
	ProviderMetadata Meta           `json:"provider_metadata,omitempty"`
	Error            string         `json:"error,omitempty"`
	StartedAt        int64          `json:"started_at,omitempty"`
	CompletedAt      int64          `json:"completed_at,omitempty"`
}

func (ToolPart) Type() string { return "tool" }

type ToolState string

const (
	ToolStatePending   ToolState = "pending"
	ToolStateRunning   ToolState = "running"
	ToolStateCompleted ToolState = "completed"
	ToolStateError     ToolState = "error"
)

// ToolCallPart is a completed tool call inside a message.
type ToolCallPart struct {
	ID               string         `json:"id,omitempty"`
	ToolUseID        string         `json:"tool_use_id,omitempty"`
	CallID           string         `json:"call_id"`
	Name             string         `json:"name"`
	Input            map[string]any `json:"input"`
	ProviderExecuted bool           `json:"provider_executed,omitempty"`
	ProviderMetadata Meta           `json:"provider_metadata,omitempty"`
}

func (ToolCallPart) Type() string { return "tool_call" }

// ToolResultPart is the outcome of a tool execution.
type ToolResultPart struct {
	ID               string `json:"id,omitempty"`
	ToolUseID        string `json:"tool_use_id,omitempty"`
	CallID           string `json:"call_id"`
	Name             string `json:"name,omitempty"`
	Output           []Part `json:"output"`
	IsError          bool   `json:"is_error"`
	Metadata         Meta   `json:"metadata,omitempty"`
	ProviderExecuted bool   `json:"provider_executed,omitempty"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
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
		ID               string            `json:"id,omitempty"`
		ToolUseID        string            `json:"tool_use_id,omitempty"`
		CallID           string            `json:"call_id"`
		Name             string            `json:"name,omitempty"`
		Output           []json.RawMessage `json:"output"`
		IsError          bool              `json:"is_error"`
		Metadata         Meta              `json:"metadata,omitempty"`
		ProviderExecuted bool              `json:"provider_executed,omitempty"`
		ProviderMetadata Meta              `json:"provider_metadata,omitempty"`
	}{p.ID, p.ToolUseID, p.CallID, p.Name, raw, p.IsError, p.Metadata, p.ProviderExecuted, p.ProviderMetadata})
}

func (p *ToolResultPart) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID               string            `json:"id,omitempty"`
		ToolUseID        string            `json:"tool_use_id,omitempty"`
		CallID           string            `json:"call_id"`
		Name             string            `json:"name,omitempty"`
		Output           []json.RawMessage `json:"output"`
		IsError          bool              `json:"is_error"`
		Metadata         Meta              `json:"metadata,omitempty"`
		ProviderExecuted bool              `json:"provider_executed,omitempty"`
		ProviderMetadata Meta              `json:"provider_metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.CallID = raw.CallID
	p.ID = raw.ID
	p.ToolUseID = raw.ToolUseID
	p.Name = raw.Name
	p.IsError = raw.IsError
	p.Metadata = raw.Metadata
	p.ProviderExecuted = raw.ProviderExecuted
	p.ProviderMetadata = raw.ProviderMetadata
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
	ID       string `json:"-"`
	Raw      []byte `json:"raw"`
}

func (p OpaquePart) Type() string { return p.TypeName }

func (p OpaquePart) MarshalJSON() ([]byte, error) {
	var raw map[string]json.RawMessage
	if p.Raw != nil {
		if err := json.Unmarshal(p.Raw, &raw); err != nil {
			return nil, err
		}
	} else {
		raw = make(map[string]json.RawMessage)
	}
	if p.TypeName != "" {
		raw["type"] = json.RawMessage(fmt.Appendf(nil, `%q`, p.TypeName))
	}
	if p.ID != "" {
		raw["id"] = json.RawMessage(fmt.Appendf(nil, `%q`, p.ID))
	}
	return json.Marshal(raw)
}

func (p *OpaquePart) UnmarshalJSON(data []byte) error {
	p.Raw = data
	var wrapper struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	p.TypeName = wrapper.Type
	p.ID = wrapper.ID
	return nil
}

// PartID returns the stable identity of a built-in part.
func PartID(p Part) string {
	switch p := p.(type) {
	case TextPart:
		return p.ID
	case ImagePart:
		return p.ID
	case ReasoningPart:
		return p.ID
	case ToolPart:
		return p.ID
	case ToolCallPart:
		return p.ID
	case ToolResultPart:
		return p.ID
	case OpaquePart:
		return p.ID
	default:
		return ""
	}
}

// WithPartID returns p with id assigned when p is a built-in part.
func WithPartID(p Part, id string) Part {
	switch p := p.(type) {
	case TextPart:
		p.ID = id
		return p
	case ImagePart:
		p.ID = id
		return p
	case ReasoningPart:
		p.ID = id
		return p
	case ToolPart:
		p.ID = id
		return p
	case ToolCallPart:
		p.ID = id
		return p
	case ToolResultPart:
		p.ID = id
		return p
	case OpaquePart:
		p.ID = id
		return p
	default:
		return p
	}
}

// ------------------------------------------------------------------
// Part registry
// ------------------------------------------------------------------

// PartUnmarshaler decodes one part JSON payload.
type PartUnmarshaler func(data []byte) (Part, error)

var builtInPartDecoders = PartDecoders{decoders: map[string]PartUnmarshaler{
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
	"tool": func(data []byte) (Part, error) {
		var p ToolPart
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
}}

// PartDecoders is an immutable generation of part decoders. Its built-in
// base is used by UnmarshalPart; plugin generations derive from that base.
type PartDecoders struct {
	decoders map[string]PartUnmarshaler
}

// BuiltinPartDecoders returns the immutable decoder generation for built-in
// parts only.
func BuiltinPartDecoders() PartDecoders { return builtInPartDecoders }

// NewPartRegistry starts a mutable registration set that builds on the
// built-in part decoders.
func NewPartRegistry() *PartRegistry { return &PartRegistry{} }

// PartRegistry collects custom part decoders for one decoder generation.
type PartRegistry struct {
	registrations []partRegistration
	registered    map[string]struct{}
	built         bool
}

type partRegistration struct {
	typeName string
	decoder  PartUnmarshaler
}

// Register adds a custom decoder. Built-in discriminator names are reserved,
// and a custom discriminator may appear only once in a registry.
func (r *PartRegistry) Register(typeName string, decoder PartUnmarshaler) error {
	if r == nil {
		return errors.New("models: nil part registry")
	}
	if r.built {
		return errors.New("models: part registry is already built")
	}
	if strings.TrimSpace(typeName) == "" {
		return errors.New("models: part type is empty")
	}
	if decoder == nil {
		return fmt.Errorf("models: part decoder %q is nil", typeName)
	}
	if _, reserved := builtInPartDecoders.decoders[typeName]; reserved {
		return fmt.Errorf("models: part type %q is reserved", typeName)
	}
	if r.registered == nil {
		r.registered = make(map[string]struct{})
	}
	if _, exists := r.registered[typeName]; exists {
		return fmt.Errorf("models: part type %q is already registered", typeName)
	}
	r.registered[typeName] = struct{}{}
	r.registrations = append(r.registrations, partRegistration{typeName: typeName, decoder: decoder})
	return nil
}

// Build freezes the registry and returns its immutable decoder generation.
func (r *PartRegistry) Build() (PartDecoders, error) {
	if r == nil {
		return PartDecoders{}, errors.New("models: nil part registry")
	}
	if r.built {
		return PartDecoders{}, errors.New("models: part registry is already built")
	}
	r.built = true
	decoders := make(map[string]PartUnmarshaler, len(builtInPartDecoders.decoders)+len(r.registrations))
	for typeName, decoder := range builtInPartDecoders.decoders {
		decoders[typeName] = decoder
	}
	for _, registration := range r.registrations {
		decoders[registration.typeName] = registration.decoder
	}
	return PartDecoders{decoders: decoders}, nil
}

// NormalizeMessages folds legacy tool-role messages into the assistant tool
// parts that produced them. It leaves no role=tool messages in the result.
func NormalizeMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Role != RoleTool {
			message.Content = normalizeToolCalls(message.Content)
			out = append(out, message)
			continue
		}
		for _, part := range message.Content {
			result, ok := part.(ToolResultPart)
			if !ok {
				continue
			}
			text := toolResultText(result)
			tool := ToolPart{ID: result.ID, ToolUseID: result.ToolUseID, CallID: result.CallID, Name: result.Name, State: ToolStateCompleted, Output: text, Metadata: result.Metadata, ProviderExecuted: result.ProviderExecuted, ProviderMetadata: result.ProviderMetadata}
			if result.IsError {
				tool.State = ToolStateError
				tool.Output = ""
				tool.Error = text
			}
			if !replaceToolPart(out, tool) {
				out = append(out, Message{Role: RoleAssistant, Content: Content{tool}})
			}
		}
	}
	return out
}

// ExpandToolMessages derives the provider-facing tool-result messages from
// canonical assistant-owned tool parts.
func ExpandToolMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages)*2)
	for _, message := range messages {
		if message.Role != RoleAssistant {
			out = append(out, message)
			continue
		}
		var results Content
		content := make(Content, 0, len(message.Content))
		hasUnresolvedTool := false
		for _, part := range message.Content {
			tool, ok := part.(ToolPart)
			if !ok {
				content = append(content, part)
				continue
			}
			if tool.State != ToolStateCompleted && tool.State != ToolStateError {
				hasUnresolvedTool = true
				continue
			}
			content = append(content, ToolCallPart{ID: tool.ID, ToolUseID: tool.ToolUseID, CallID: tool.CallID, Name: tool.Name, Input: tool.Input, ProviderExecuted: tool.ProviderExecuted, ProviderMetadata: tool.ProviderMetadata})
			text := tool.Output
			if tool.State == ToolStateError {
				if tool.Output != "" && tool.Error != "" {
					text = tool.Output + "\n" + tool.Error
				} else if tool.Error != "" {
					text = tool.Error
				}
			}
			results = append(results, ToolResultPart{ID: tool.ID, ToolUseID: tool.ToolUseID, CallID: tool.CallID, Name: tool.Name, Output: Content{TextPart{Text: text}}, IsError: tool.State == ToolStateError, Metadata: tool.Metadata, ProviderExecuted: tool.ProviderExecuted, ProviderMetadata: tool.ProviderMetadata})
		}
		if hasUnresolvedTool && len(content) == 0 {
			continue
		}
		message.Content = content
		out = append(out, message)
		if len(results) > 0 {
			out = append(out, Message{Role: RoleTool, Content: results})
		}
	}
	return out
}

func normalizeToolCalls(content Content) Content {
	out := make(Content, 0, len(content))
	for _, part := range content {
		if call, ok := part.(ToolCallPart); ok {
			out = append(out, ToolPart{ID: call.ID, ToolUseID: call.ToolUseID, CallID: call.CallID, Name: call.Name, State: ToolStatePending, Input: call.Input, ProviderExecuted: call.ProviderExecuted, ProviderMetadata: call.ProviderMetadata})
			continue
		}
		out = append(out, part)
	}
	return out
}

func replaceToolPart(messages []Message, tool ToolPart) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != RoleAssistant {
			continue
		}
		for j, part := range messages[i].Content {
			current, ok := part.(ToolPart)
			if !ok {
				continue
			}
			if tool.ToolUseID != "" {
				if current.ToolUseID != tool.ToolUseID {
					continue
				}
			} else if current.CallID != tool.CallID {
				continue
			}
			tool.Input = current.Input
			tool.InputRaw = current.InputRaw
			tool.StartedAt = current.StartedAt
			if current.ID != "" {
				tool.ID = current.ID
			}
			if tool.ToolUseID == "" {
				tool.ToolUseID = current.ToolUseID
			}
			if !tool.ProviderExecuted {
				tool.ProviderExecuted = current.ProviderExecuted
			}
			if tool.ProviderMetadata == nil {
				tool.ProviderMetadata = current.ProviderMetadata
			}
			messages[i].Content[j] = tool
			return true
		}
	}
	return false
}

func toolResultText(part ToolResultPart) string {
	var out string
	for _, item := range part.Output {
		if text, ok := item.(TextPart); ok {
			out += text.Text
		}
	}
	return out
}

func MarshalPart(p Part) ([]byte, error) {
	if op, ok := p.(OpaquePart); ok {
		return json.Marshal(op)
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
	return builtInPartDecoders.UnmarshalPart(data)
}

// UnmarshalPart decodes a part using this decoder generation. Unknown part
// types remain OpaquePart values so newer payloads can round-trip safely.
func (d PartDecoders) UnmarshalPart(data []byte) (Part, error) {
	var wrapper struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	fn, ok := d.decoders[wrapper.Type]
	if !ok {
		var p OpaquePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
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
// use provider-qualified model refs such as "openai/gpt-5.6-terra" instead of
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
	FinishReasonBlocked   FinishReason = "blocked"
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
// CallTrace
// ------------------------------------------------------------------

// CallTrace is a safe, versioned structural snapshot of a model call.
// It contains only structural information and never credentials,
// request text, message content, raw tool output, HTTP headers, or
// raw payloads.
type CallTrace struct {
	Version      string         `json:"version"`
	Model        ModelRef       `json:"model"`
	API          API            `json:"api"`
	Provider     string         `json:"provider"`
	Capabilities Capabilities   `json:"capabilities"`
	Runtime      RuntimeTrace   `json:"runtime"`
	Tools        []ToolTrace    `json:"tools,omitempty"`
	Messages     MessageTrace   `json:"messages"`
	System       SystemTrace    `json:"system"`
	Lowered      LoweredOptions `json:"lowered,omitempty"`
}

type RuntimeTrace struct {
	CurrentDate bool `json:"current_date"`
}

type ToolTrace struct {
	Name        string `json:"name"`
	SchemaHash  string `json:"schema_hash"`
	SchemaBytes int    `json:"schema_bytes"`
}

type MessageTrace struct {
	Count     int            `json:"count"`
	ByRole    map[string]int `json:"by_role"`
	PartKinds map[string]int `json:"part_kinds"`
}

type SystemTrace struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// LoweredOptions captures provider-calculated lowered request flags.
type LoweredOptions struct {
	ReasoningSummaryAuto bool `json:"reasoning_summary_auto,omitempty"`
}

// NewCallTrace builds a CallTrace from a Request and provider lowering info.
func NewCallTrace(req Request, lowered LoweredOptions) CallTrace {
	tools := make([]ToolTrace, len(req.Tools))
	for i, t := range req.Tools {
		schemaJSON, _ := json.Marshal(t.InputSchema)
		tools[i] = ToolTrace{
			Name:        t.Name,
			SchemaHash:  sha256hex(schemaJSON),
			SchemaBytes: len(schemaJSON),
		}
	}

	byRole := make(map[string]int)
	partKinds := make(map[string]int)
	for _, m := range req.Messages {
		byRole[string(m.Role)]++
		for _, p := range m.Content {
			partKinds[p.Type()]++
		}
	}

	sysBytes := []byte(req.System)
	return CallTrace{
		Version:      "1",
		Model:        req.Model,
		API:          req.Model.API,
		Provider:     req.Model.Provider,
		Capabilities: req.Capabilities,
		Runtime: RuntimeTrace{
			CurrentDate: strings.Contains(req.System, "Current date:"),
		},
		Tools:    tools,
		Messages: MessageTrace{Count: len(req.Messages), ByRole: byRole, PartKinds: partKinds},
		System:   SystemTrace{SHA256: sha256hex(sysBytes), Bytes: len(sysBytes)},
		Lowered:  lowered,
	}
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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
