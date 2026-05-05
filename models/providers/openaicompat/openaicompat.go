// Package openaicompat implements models.Model for any provider that
// speaks the OpenAI Chat Completions wire format (/v1/chat/completions).
//
// This includes: OpenAI (non-Responses), Groq, DeepSeek, OpenRouter, Mistral,
// Ollama-OpenAI-compat, and OpenCode Zen (for non-Anthropic models).
//
// Wire reference: https://platform.openai.com/docs/api-reference/chat/streaming
//
// Stream mapping (Chat Completions SSE -> models.StreamPart):
//
//	[STREAM START]                        -> StreamStartPart{}
//	choices[0].delta.content (first)      -> TextStartPart{id}
//	choices[0].delta.content              -> TextDeltaPart{id, delta}
//	choices[0].delta.reasoning_content    -> ReasoningDeltaPart{id, delta} (DeepSeek/Qwen)
//	choices[0].delta.tool_calls[n] start  -> ToolInputStartPart{id=call_id, tool_name}
//	choices[0].delta.tool_calls[n] delta  -> ToolInputDeltaPart{id=call_id, delta}
//	choices[0].finish_reason != null      -> TextEndPart / ToolInputEndPart + ToolCallPart_ + FinishPart
//
// ToolChoice mapping:
//
//	ToolChoiceAuto     -> "auto"
//	ToolChoiceRequired -> "required"
//	ToolChoiceNone     -> "none"
//	ToolChoiceTool     -> {"type":"function","function":{"name":"<tool>"}}
//
// Reasoning: for providers that surface reasoning via delta.reasoning_content
// (DeepSeek-R1, Qwen QwQ) we capture it. No special request param is needed.
// The Thinking config's Effort field is NOT forwarded (no standard param).
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/transform"
)

// Config controls construction of a Client.
type Config struct {
	// ProviderID is the registry key (e.g. "openai", "opencodezen", "deepseek").
	ProviderID string
	// CatalogProvider is the catalog lookup key. Falls back to ProviderID if empty.
	CatalogProvider string
	APIKey     string
	Model      string
	BaseURL    string
	MaxTokens  int
	MaxRetries int
	Options    map[string]any
}

const (
	defaultMaxTokens  = 4096
	defaultMaxRetries = 3
	httpTimeout       = 5 * time.Minute
	maxRetryDelay     = 60 * time.Second
)

// Client is a configured OpenAI-compatible Model.
type Client struct {
	providerID      string
	catalogProvider string
	apiKey          string
	model           string
	baseURL         string
	maxTokens       int
	httpClient      *http.Client
	maxRetries      int
}

// New constructs a Client. API key resolution order: Config.APIKey,
// Options["api_key"], then <PROVIDER_ID_UPPER>_API_KEY env var.
func New(cfg Config) (*Client, error) {
	providerID := cfg.ProviderID
	if providerID == "" {
		providerID = "openaicompat"
	}
	catalogProvider := cfg.CatalogProvider
	if catalogProvider == "" {
		catalogProvider = providerID
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		if k, ok := cfg.Options["api_key"].(string); ok && k != "" {
			apiKey = k
		}
	}
	if apiKey == "" {
		envKey := strings.ToUpper(strings.ReplaceAll(providerID, "-", "_")) + "_API_KEY"
		apiKey = os.Getenv(envKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%s: no API key (set Options[\"api_key\"] or %s_API_KEY)",
			providerID, strings.ToUpper(strings.ReplaceAll(providerID, "-", "_")))
	}

	model := cfg.Model
	if model == "" {
		if m, ok := cfg.Options["model"].(string); ok && m != "" {
			model = m
		}
	}
	if model == "" {
		return nil, fmt.Errorf("%s: no model specified", providerID)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if u, ok := cfg.Options["base_url"].(string); ok && u != "" {
			baseURL = u
		}
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%s: no base_url specified", providerID)
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		if v, ok := cfg.Options["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				maxTokens = n
			case float64:
				maxTokens = int(n)
			}
		}
	}
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	return &Client{
		providerID:      providerID,
		catalogProvider: catalogProvider,
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		maxTokens:       maxTokens,
		httpClient:      &http.Client{Timeout: httpTimeout},
		maxRetries:      maxRetries,
	}, nil
}

// Info returns catalog ModelInfo. API is always APIOpenAICompletions.
func (c *Client) Info() models.ModelInfo {
	if info, ok := catalog.Get(c.catalogProvider, c.model); ok {
		info.API = models.APIOpenAICompletions
		info.BaseURL = c.baseURL
		return info
	}
	return models.ModelInfo{
		Provider: c.providerID,
		ID:       c.model,
		API:      models.APIOpenAICompletions,
		BaseURL:  c.baseURL,
	}
}

func (c *Client) origin() *models.MessageOrigin {
	return &models.MessageOrigin{
		Provider: c.providerID,
		API:      models.APIOpenAICompletions,
		ModelID:  c.model,
	}
}

// CountTokens returns a char-based approximation (4 chars ≈ 1 token).
func (c *Client) CountTokens(_ context.Context, msgs []models.Message) (int, error) {
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
	return total / 4, nil
}

// Stream starts a streaming Chat Completions request.
func (c *Client) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	info := c.Info()
	req.Messages = transform.Apply(req.Messages, transform.Target{
		Provider:     c.providerID,
		API:          models.APIOpenAICompletions,
		ModelID:      c.model,
		Capabilities: info.Capabilities,
	})

	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", c.providerID, err)
	}

	resp, err := c.doStreamWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	out := models.NewEventStream[models.StreamPart, *models.Message](64)
	go runStream(ctx, resp, out, c.origin(), c.providerID)
	return out, nil
}

// ---- request building ------------------------------------------------------

type chatRequest struct {
	Model          string              `json:"model"`
	Messages       []chatMessage       `json:"messages"`
	Stream         bool                `json:"stream"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Tools          []chatTool          `json:"tools,omitempty"`
	ToolChoice     any                 `json:"tool_choice,omitempty"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

// chatResponseFormat carries OpenAI Chat Completions structured-output config.
type chatResponseFormat struct {
	Type       string              `json:"type"` // "json_schema"
	JSONSchema *chatJSONSchemaSpec `json:"json_schema,omitempty"`
}

type chatJSONSchemaSpec struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"` // string or []chatContentPart
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
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
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string `json:"type"` // always "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func (c *Client) buildRequest(req models.Request) chatRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = c.maxTokens
	}

	r := chatRequest{
		Model:     c.model,
		Messages:  c.toWireMessages(req),
		Stream:    true,
		MaxTokens: maxOut,
	}

	// Tools
	if len(req.Tools) > 0 {
		r.Tools = make([]chatTool, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i].Type = "function"
			r.Tools[i].Function.Name = t.Name
			r.Tools[i].Function.Description = t.Description
			r.Tools[i].Function.Parameters = t.InputSchema
		}
	}

	// ToolChoice (only when tools present)
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case models.ToolChoiceRequired:
			r.ToolChoice = "required"
		case models.ToolChoiceNone:
			r.ToolChoice = "none"
		case models.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = map[string]any{
					"type": "function",
					"function": map[string]any{"name": req.ToolChoice.Tool},
				}
			}
		case models.ToolChoiceAuto:
			r.ToolChoice = "auto"
		// zero-value: omit; provider default is "auto"
		}
	}

	// Structured output (response_format=json_schema)
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

// toWireMessages converts models messages to OpenAI Chat Completions format.
//
// Role mapping:
//   - user      -> {role:"user", content:[{type:"text",text},...]}
//   - assistant -> {role:"assistant", content:"...", tool_calls:[...]}
//   - tool      -> {role:"tool", tool_call_id, content} (one per ToolResultPart)
func (c *Client) toWireMessages(req models.Request) []chatMessage {
	var out []chatMessage

	if req.System != "" {
		out = append(out, chatMessage{Role: "system", Content: req.System})
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
			// If it's only text parts, simplify to a plain string.
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
					// Drop reasoning on replay — can't round-trip via chat completions.
				case models.ToolCallPart:
					args, _ := json.Marshal(v.Input)
					callID, _ := splitCompositeID(v.CallID)
					tc := chatToolCall{
						ID:   callID,
						Type: "function",
					}
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
					content := toolResultText(tr.Output)
					out = append(out, chatMessage{
						Role:       "tool",
						ToolCallID: callID,
						Content:    content,
					})
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

// ---- stream parsing --------------------------------------------------------

type streamParser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	terminated bool
	origin     *models.MessageOrigin
	providerID string

	// In-flight text assembly
	textID       string
	textBuf      strings.Builder
	textStarted  bool
	// Reasoning assembly (delta.reasoning_content — DeepSeek-style)
	reasoningID      string
	reasoningBuf     strings.Builder
	reasoningStarted bool
	// Tool call assembly keyed by index
	toolCalls map[int]*inFlightToolCall

	// Final assembly
	content      models.Content
	finishReason string
	usage        models.Usage
	responseMeta models.ResponseMetadata
	startEmitted bool
}

type inFlightToolCall struct {
	id       string // call_id from first chunk
	name     string
	jsonBuf  strings.Builder
	streamID string // models stream-part id
}

func newStreamParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, providerID string) *streamParser {
	return &streamParser{
		out:        out,
		origin:     origin,
		providerID: providerID,
		toolCalls:  make(map[int]*inFlightToolCall),
	}
}

// chunkDelta is the relevant subset of a Chat Completions delta chunk.
type chunkDelta struct {
	Role             string          `json:"role"`
	Content          *string         `json:"content"`
	ReasoningContent *string         `json:"reasoning_content"` // DeepSeek/Qwen
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
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
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

	// Reasoning delta (DeepSeek-R1 / Qwen QwQ style)
	if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
		if !p.reasoningStarted {
			p.reasoningID = "reasoning_0"
			p.out.Push(models.ReasoningStartPart{ID: p.reasoningID})
			p.reasoningStarted = true
		}
		p.reasoningBuf.WriteString(*delta.ReasoningContent)
		p.out.Push(models.ReasoningDeltaPart{ID: p.reasoningID, Delta: *delta.ReasoningContent})
	}

	// Text delta
	if delta.Content != nil && *delta.Content != "" {
		if !p.textStarted {
			// Close reasoning if it was open (reasoning always comes before text)
			if p.reasoningStarted {
				p.content = append(p.content, models.ReasoningPart{
					Reasoning: p.reasoningBuf.String(),
					// No Signature for completions-style; can't round-trip.
				})
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

	// Tool call deltas
	for _, tc := range delta.ToolCalls {
		if _, exists := p.toolCalls[tc.Index]; !exists {
			streamID := "tool_" + strconv.Itoa(tc.Index)
			p.toolCalls[tc.Index] = &inFlightToolCall{
				id:       tc.ID,
				name:     tc.Function.Name,
				streamID: streamID,
			}
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

	// Finish
	if choice.FinishReason != nil {
		p.finishReason = *choice.FinishReason
		p.flush()
		p.terminateNormal()
		return true
	}

	return false
}

// flush closes any open text/tool blocks and populates p.content.
func (p *streamParser) flush() {
	if p.reasoningStarted {
		p.content = append(p.content, models.ReasoningPart{
			Reasoning: p.reasoningBuf.String(),
		})
		p.out.Push(models.ReasoningEndPart{ID: p.reasoningID})
		p.reasoningStarted = false
	}
	if p.textStarted {
		p.content = append(p.content, models.TextPart{Text: p.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: p.textID})
		p.textStarted = false
	}
	// Tool calls: emit end + assembled ToolCallPart in index order.
	// Iterate sorted keys rather than 0..len because providers may send
	// non-contiguous indices (e.g., 0 and 2 with no 1); using len(map) as
	// the upper bound would silently drop trailing calls.
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
				p.out.Push(models.ErrorPart{
					Message: fmt.Sprintf("%s: tool input json: %v", p.providerID, err),
					Code:    "invalid_tool_input",
				})
			}
		}
		p.content = append(p.content, models.ToolCallPart{
			CallID: call.id,
			Name:   call.name,
			Input:  input,
		})
		p.out.Push(models.ToolInputEndPart{ID: call.id})
		p.out.Push(models.ToolCallPart_{ID: call.id, ToolName: call.name, Input: input})
	}
}

func (p *streamParser) terminateNormal() {
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens + p.usage.CachedInputTokens
	reason := mapFinishReason(p.finishReason)
	msg := &models.Message{
		Role:         models.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: reason,
	}
	p.out.Push(models.FinishPart{
		Reason:   reason,
		Usage:    p.usage,
		Message:  msg,
		Metadata: p.responseMeta,
	})
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
	msg := &models.Message{
		Role:         models.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: reason,
	}
	p.out.Push(models.FinishPart{
		Reason:   reason,
		Usage:    p.usage,
		Message:  msg,
		Metadata: p.responseMeta,
	})
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
				// Some providers send [DONE] without a finish_reason chunk.
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

// ---- HTTP plumbing ---------------------------------------------------------

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func (c *Client) doStreamWithRetry(ctx context.Context, body []byte) (*http.Response, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.doRequest(ctx, body)
		if err != nil {
			if shouldRetryTransport(err) && attempt < c.maxRetries {
				if waitErr := waitForRetry(ctx, backoffDelay(attempt)); waitErr != nil {
					return nil, fmt.Errorf("%s: %w", c.providerID, waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("%s: request: %w", c.providerID, err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if shouldRetryStatus(resp.StatusCode) && attempt < c.maxRetries {
			if waitErr := waitForRetry(ctx, retryDelay(resp, attempt)); waitErr != nil {
				return nil, fmt.Errorf("%s: API %d: %s", c.providerID, resp.StatusCode, string(raw))
			}
			continue
		}
		return nil, fmt.Errorf("%s: API %d: %s", c.providerID, resp.StatusCode, string(raw))
	}
	return nil, fmt.Errorf("%s: retries exhausted", c.providerID)
}

func (c *Client) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.httpClient.Do(req)
}

func shouldRetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

func shouldRetryTransport(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	return true
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return d
		}
	}
	return backoffDelay(attempt)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	if seconds, err := time.ParseDuration(v + "s"); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > maxRetryDelay {
			seconds = maxRetryDelay
		}
		return seconds, true
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, false
		}
		if d > maxRetryDelay {
			d = maxRetryDelay
		}
		return d, true
	}
	return 0, false
}

func backoffDelay(attempt int) time.Duration {
	d := time.Second * time.Duration(2<<attempt)
	if d > maxRetryDelay {
		return maxRetryDelay
	}
	return d
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RegisterProvider registers an OpenAI-compatible provider with the default
// registry. The factory reads model, base_url, api_key from opts.
// catalogProvider overrides the catalog lookup key (defaults to providerID).
func RegisterProvider(providerID, catalogProvider, defaultBaseURL, envKeyVar string, authTypes []provider.AuthType) {
	provider.Register(provider.ProviderMeta{
		ID:        providerID,
		Name:      providerID,
		AuthTypes: authTypes,
		Factory: func(opts map[string]any) (models.Model, error) {
			apiKey := ""
			if k, ok := opts["api_key"].(string); ok {
				apiKey = k
			}
			if apiKey == "" {
				apiKey = os.Getenv(envKeyVar)
			}
			model := ""
			if m, ok := opts["model"].(string); ok {
				model = m
			}
			baseURL := defaultBaseURL
			if u, ok := opts["base_url"].(string); ok && u != "" {
				baseURL = u
			}
			return New(Config{
				ProviderID:      providerID,
				CatalogProvider: catalogProvider,
				APIKey:          apiKey,
				Model:           model,
				BaseURL:         baseURL,
				Options:         opts,
			})
		},
	})
}

// Compile-time check.
var _ models.Model = (*Client)(nil)
