// Package openaichat implements the OpenAI Chat Completions protocol family.
// It is reusable by OpenAI-compatible providers that share the
// /chat/completions request and SSE streaming shape.
package openaichat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/models/transform"
)

// Protocol adapts models.Request values to OpenAI Chat Completions requests
// and parses OpenAI Chat-compatible SSE streams back into models.StreamPart.
type Protocol struct{}

func (Protocol) API() models.API { return models.APIOpenAICompletions }

func (Protocol) Prepare(_ context.Context, ref route.ModelRef, req models.Request) (*route.PreparedBody, error) {
	profile := profileFromRef(ref)
	req.Messages = transform.Apply(req.Messages, transform.Target{
		Provider:     ref.Provider,
		API:          models.APIOpenAICompletions,
		ModelID:      ref.ModelID,
		Capabilities: ref.Info.Capabilities,
	})
	payload, err := marshalPayload(buildRequest(ref, req, profile), ref, req)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", ref.Provider, err)
	}
	return &route.PreparedBody{Body: payload}, nil
}

func (Protocol) ParseStream(ctx context.Context, ref route.ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message]) {
	runStream(ctx, resp, out, origin(ref), ref.Provider)
}

func (Protocol) CountTokens(_ context.Context, _ route.ModelRef, msgs []models.Message) (int, error) {
	return CountTokens(msgs), nil
}

// CountTokens returns a char-based approximation (4 chars ~= 1 token).
func CountTokens(msgs []models.Message) int {
	total := 0
	for _, m := range msgs {
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				total += len(v.Text)
			case models.ReasoningPart:
				total += len(v.Reasoning)
			case models.ToolCallPart:
				total += len(v.Name) + 8
				if b, err := json.Marshal(v.Input); err == nil {
					total += len(b)
				}
			case models.ToolResultPart:
				for _, rp := range v.Output {
					if t, ok := rp.(models.TextPart); ok {
						total += len(t.Text)
					}
				}
			}
		}
	}
	return total / 4
}

func origin(ref route.ModelRef) *models.MessageOrigin {
	return &models.MessageOrigin{
		Provider: ref.Provider,
		API:      models.APIOpenAICompletions,
		ModelID:  ref.ModelID,
	}
}

type chatRequest struct {
	Model               string              `json:"model"`
	Messages            []chatMessage       `json:"messages"`
	Stream              bool                `json:"stream"`
	MaxTokens           int                 `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                 `json:"max_completion_tokens,omitempty"`
	Store               *bool               `json:"store,omitempty"`
	Tools               []chatTool          `json:"tools,omitempty"`
	ToolChoice          any                 `json:"tool_choice,omitempty"`
	ResponseFormat      *chatResponseFormat `json:"response_format,omitempty"`
}

// Profile describes service-specific Chat Completions dialect choices.
type Profile struct {
	ID                    string
	SystemRole            string
	MaxTokensField        string
	Store                 *bool
	ReasoningContentField string
}

const (
	ProfileOpenAIChat  = "openai.chat"
	ProfileCompatChat  = "openai-compatible.chat"
	ProfileOpenCodeZen = "opencode-zen.openai-chat"
)

func KnownProfile(id string) (Profile, bool) {
	switch id {
	case ProfileOpenAIChat:
		store := false
		return Profile{ID: id, SystemRole: "developer", MaxTokensField: "max_completion_tokens", Store: &store, ReasoningContentField: "reasoning_content"}, true
	case ProfileOpenCodeZen:
		return Profile{ID: id, SystemRole: "system", MaxTokensField: "max_tokens", ReasoningContentField: "reasoning_content"}, true
	case "", ProfileCompatChat:
		return Profile{ID: ProfileCompatChat, SystemRole: "system", MaxTokensField: "max_tokens", ReasoningContentField: "reasoning_content"}, true
	default:
		return Profile{}, false
	}
}

func profileFromRef(ref route.ModelRef) Profile {
	switch v := ref.Compat.(type) {
	case Profile:
		return v
	case *Profile:
		if v != nil {
			return *v
		}
	case string:
		if profile, ok := KnownProfile(v); ok {
			return profile
		}
	}
	profile, _ := KnownProfile("")
	return profile
}

type chatResponseFormat struct {
	Type       string              `json:"type"`
	JSONSchema *chatJSONSchemaSpec `json:"json_schema,omitempty"`
}

type chatJSONSchemaSpec struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type chatContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func buildRequest(ref route.ModelRef, req models.Request, profile Profile) chatRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = ref.MaxOutputTokens
	}
	r := chatRequest{
		Model:    ref.ModelID,
		Messages: toWireMessages(req, profile),
		Stream:   true,
		Store:    profile.Store,
	}
	if profile.MaxTokensField == "max_completion_tokens" {
		r.MaxCompletionTokens = maxOut
	} else {
		r.MaxTokens = maxOut
	}
	if len(req.Tools) > 0 {
		r.Tools = make([]chatTool, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i].Type = "function"
			r.Tools[i].Function.Name = t.Name
			r.Tools[i].Function.Description = t.Description
			r.Tools[i].Function.Parameters = t.InputSchema
		}
	}
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case models.ToolChoiceRequired:
			r.ToolChoice = "required"
		case models.ToolChoiceNone:
			r.ToolChoice = "none"
		case models.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = map[string]any{
					"type":     "function",
					"function": map[string]any{"name": req.ToolChoice.Tool},
				}
			}
		case models.ToolChoiceAuto:
			r.ToolChoice = "auto"
		}
	}
	if req.OutputSchema != nil {
		name := req.OutputSchema.Name
		if name == "" {
			name = "response"
		}
		r.ResponseFormat = &chatResponseFormat{
			Type: "json_schema",
			JSONSchema: &chatJSONSchemaSpec{
				Name:   name,
				Schema: req.OutputSchema.Schema,
				Strict: req.OutputSchema.Strict,
			},
		}
	}
	return r
}

func marshalPayload(r chatRequest, ref route.ModelRef, req models.Request) ([]byte, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for _, key := range []string{ref.Provider, string(models.APIOpenAICompletions)} {
		for k, v := range req.ProviderOptions[key] {
			payload[k] = v
		}
	}
	return json.Marshal(payload)
}

func toWireMessages(req models.Request, profile Profile) []chatMessage {
	var out []chatMessage
	if req.System != "" {
		role := profile.SystemRole
		if role == "" {
			role = "system"
		}
		out = append(out, chatMessage{Role: role, Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case models.RoleUser:
			var parts []chatContentPart
			for _, p := range m.Content {
				switch v := p.(type) {
				case models.TextPart:
					if strings.TrimSpace(v.Text) == "" {
						continue
					}
					parts = append(parts, chatContentPart{Type: "text", Text: v.Text})
				case models.ImagePart:
					parts = append(parts, chatContentPart{
						Type: "image_url",
						ImageURL: &struct {
							URL    string `json:"url"`
							Detail string `json:"detail,omitempty"`
						}{
							URL:    "data:" + v.MimeType + ";base64," + v.Data,
							Detail: "auto",
						},
					})
				}
			}
			if len(parts) == 0 {
				continue
			}
			if len(parts) == 1 && parts[0].Type == "text" {
				out = append(out, chatMessage{Role: "user", Content: parts[0].Text})
			} else {
				out = append(out, chatMessage{Role: "user", Content: parts})
			}
		case models.RoleAssistant:
			msg := chatMessage{Role: "assistant"}
			var textBuf strings.Builder
			var toolCalls []chatToolCall
			for _, p := range m.Content {
				switch v := p.(type) {
				case models.TextPart:
					textBuf.WriteString(v.Text)
				case models.ReasoningPart:
				case models.ToolCallPart:
					args, _ := json.Marshal(v.Input)
					callID, _ := splitCompositeID(v.CallID)
					tc := chatToolCall{ID: callID, Type: "function"}
					tc.Function.Name = v.Name
					tc.Function.Arguments = string(args)
					toolCalls = append(toolCalls, tc)
				}
			}
			msg.Content = textBuf.String()
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			if msg.Content == "" && len(msg.ToolCalls) == 0 {
				continue
			}
			out = append(out, msg)
		case models.RoleTool:
			for _, p := range m.Content {
				if tr, ok := p.(models.ToolResultPart); ok {
					callID, _ := splitCompositeID(tr.CallID)
					out = append(out, chatMessage{Role: "tool", ToolCallID: callID, Content: toolResultText(tr.Output)})
				}
			}
		}
	}
	return out
}

func toolResultText(out []models.Part) string {
	var sb strings.Builder
	for _, p := range out {
		if t, ok := p.(models.TextPart); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

func splitCompositeID(id string) (callID, itemID string) {
	if i := strings.IndexByte(id, '|'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

type streamParser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	terminated bool
	origin     *models.MessageOrigin
	providerID string

	textID      string
	textBuf     strings.Builder
	textStarted bool

	reasoningID      string
	reasoningBuf     strings.Builder
	reasoningStarted bool
	toolCalls        map[int]*inFlightToolCall

	content      models.Content
	finishReason string
	usage        models.Usage
	responseMeta models.ResponseMetadata
	startEmitted bool
}

type inFlightToolCall struct {
	id      string
	name    string
	jsonBuf strings.Builder
}

func newStreamParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, providerID string) *streamParser {
	return &streamParser{out: out, origin: origin, providerID: providerID, toolCalls: make(map[int]*inFlightToolCall)}
}

type chunkDelta struct {
	Role             string          `json:"role"`
	Content          *string         `json:"content"`
	ReasoningContent *string         `json:"reasoning_content"`
	ToolCalls        []deltaToolCall `json:"tool_calls"`
}

type deltaToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (p *streamParser) handleChunk(data string) bool {
	var chunk struct {
		ID      string `json:"id"`
		Choices []struct {
			Delta        chunkDelta `json:"delta"`
			FinishReason *string    `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		p.terminateError(fmt.Errorf("%s: parse chunk: %w", p.providerID, err))
		return true
	}
	if !p.startEmitted && chunk.ID != "" {
		p.responseMeta = models.ResponseMetadata{ID: chunk.ID}
		p.out.Push(models.StreamStartPart{})
		p.out.Push(models.ResponseMetadataPart{ResponseMetadata: p.responseMeta})
		p.startEmitted = true
	}
	if chunk.Usage != nil {
		cached := 0
		if chunk.Usage.PromptTokensDetails != nil {
			cached = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		reasoning := 0
		if chunk.Usage.CompletionTokensDetails != nil {
			reasoning = chunk.Usage.CompletionTokensDetails.ReasoningTokens
		}
		p.usage = models.Usage{
			InputTokens:       chunk.Usage.PromptTokens - cached,
			OutputTokens:      chunk.Usage.CompletionTokens,
			TotalTokens:       chunk.Usage.TotalTokens,
			CachedInputTokens: cached,
			ReasoningTokens:   reasoning,
		}
	}
	if len(chunk.Choices) == 0 {
		return false
	}
	choice := chunk.Choices[0]
	delta := choice.Delta
	if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
		if !p.reasoningStarted {
			p.reasoningID = "reasoning_0"
			p.out.Push(models.ReasoningStartPart{ID: p.reasoningID})
			p.reasoningStarted = true
		}
		p.reasoningBuf.WriteString(*delta.ReasoningContent)
		p.out.Push(models.ReasoningDeltaPart{ID: p.reasoningID, Delta: *delta.ReasoningContent})
	}
	if delta.Content != nil && *delta.Content != "" {
		if !p.textStarted {
			if p.reasoningStarted {
				p.content = append(p.content, models.ReasoningPart{Reasoning: p.reasoningBuf.String()})
				p.out.Push(models.ReasoningEndPart{ID: p.reasoningID})
				p.reasoningStarted = false
			}
			p.textID = "text_0"
			p.out.Push(models.TextStartPart{ID: p.textID})
			p.textStarted = true
		}
		p.textBuf.WriteString(*delta.Content)
		p.out.Push(models.TextDeltaPart{ID: p.textID, Delta: *delta.Content})
	}
	for _, tc := range delta.ToolCalls {
		if _, exists := p.toolCalls[tc.Index]; !exists {
			p.toolCalls[tc.Index] = &inFlightToolCall{id: tc.ID, name: tc.Function.Name}
			p.out.Push(models.ToolInputStartPart{ID: tc.ID, ToolName: tc.Function.Name})
		}
		call := p.toolCalls[tc.Index]
		if tc.ID != "" && call.id == "" {
			call.id = tc.ID
		}
		if tc.Function.Name != "" && call.name == "" {
			call.name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			call.jsonBuf.WriteString(tc.Function.Arguments)
			p.out.Push(models.ToolInputDeltaPart{ID: call.id, Delta: tc.Function.Arguments})
		}
	}
	if choice.FinishReason != nil {
		p.finishReason = *choice.FinishReason
		p.flush()
		p.terminateNormal()
		return true
	}
	return false
}

func (p *streamParser) flush() {
	if p.reasoningStarted {
		p.content = append(p.content, models.ReasoningPart{Reasoning: p.reasoningBuf.String()})
		p.out.Push(models.ReasoningEndPart{ID: p.reasoningID})
		p.reasoningStarted = false
	}
	if p.textStarted {
		p.content = append(p.content, models.TextPart{Text: p.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: p.textID})
		p.textStarted = false
	}
	keys := make([]int, 0, len(p.toolCalls))
	for k := range p.toolCalls {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, i := range keys {
		call := p.toolCalls[i]
		var input map[string]any
		if call.jsonBuf.Len() > 0 {
			if err := json.Unmarshal([]byte(call.jsonBuf.String()), &input); err != nil {
				p.out.Push(models.ErrorPart{Message: fmt.Sprintf("%s: tool input json: %v", p.providerID, err), Code: "invalid_tool_input"})
			}
		}
		p.content = append(p.content, models.ToolCallPart{CallID: call.id, Name: call.name, Input: input})
		p.out.Push(models.ToolInputEndPart{ID: call.id})
		p.out.Push(models.ToolCallPart_{ID: call.id, ToolName: call.name, Input: input})
	}
}

func (p *streamParser) terminateNormal() {
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens + p.usage.CachedInputTokens
	reason := mapFinishReason(p.finishReason)
	msg := &models.Message{Role: models.RoleAssistant, Content: p.content, Origin: p.origin, FinishReason: reason}
	p.out.Push(models.FinishPart{Reason: reason, Usage: p.usage, Message: msg, Metadata: p.responseMeta})
	p.close(msg, nil)
}

func (p *streamParser) terminateError(err error) {
	if p.terminated {
		return
	}
	if !p.startEmitted {
		p.out.Push(models.StreamStartPart{})
	}
	p.out.Push(models.ErrorPart{Message: err.Error()})
	p.terminate(models.FinishReasonError, err)
}

func (p *streamParser) terminateAborted() {
	if p.terminated {
		return
	}
	p.terminate(models.FinishReasonAborted, context.Canceled)
}

func (p *streamParser) terminate(reason models.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
	msg := &models.Message{Role: models.RoleAssistant, Content: p.content, Origin: p.origin, FinishReason: reason}
	p.out.Push(models.FinishPart{Reason: reason, Usage: p.usage, Message: msg, Metadata: p.responseMeta})
	p.close(msg, err)
}

func (p *streamParser) close(msg *models.Message, err error) {
	p.terminated = true
	p.out.Close(msg, err)
}

func mapFinishReason(r string) models.FinishReason {
	switch r {
	case "stop":
		return models.FinishReasonStop
	case "length":
		return models.FinishReasonLength
	case "tool_calls", "function_call":
		return models.FinishReasonToolCalls
	case "content_filter":
		return models.FinishReasonContentFilter
	case "":
		return models.FinishReasonUnknown
	default:
		return models.FinishReasonOther
	}
}

func runStream(ctx context.Context, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, providerID string) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	p := newStreamParser(out, origin, providerID)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			p.terminateAborted()
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if !p.terminated {
				p.flush()
				p.terminateNormal()
			}
			return
		}
		if done := p.handleChunk(data); done {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		p.terminateError(fmt.Errorf("%s: scanner: %w", providerID, err))
		return
	}
	p.terminateError(fmt.Errorf("%s: stream closed without [DONE]", providerID))
}

var _ route.Protocol = Protocol{}
