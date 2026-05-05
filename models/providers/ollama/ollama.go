// Package ollama implements models.Model for a local Ollama server.
//
// Wire reference: https://github.com/ollama/ollama/blob/main/docs/api.md
//
// Stream model: Ollama's /api/chat with stream=true emits one JSON object per
// line. Each object carries an incremental message delta and, on the final
// line, done=true with cumulative usage stats. Ollama does not stream tool
// arguments; tool calls appear fully formed when the model emits one. We
// therefore synthesize the three-phase tool flow at end-of-stream:
//
//	tool-input-start (id, name)
//	tool-input-delta (id, full json)
//	tool-input-end   (id)
//	tool-call        (id, name, parsed input)
//
// Token counting: Ollama doesn't expose a tokenizer endpoint. CountTokens
// returns a chars/4 heuristic, conservative enough to drive the compaction
// threshold without unfairly penalizing short messages.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/transform"
)

// Meta is the registry entry for the Ollama provider. No auth (Ollama runs
// locally by default).
var Meta = provider.ProviderMeta{
	ID:        "ollama",
	Name:      "Ollama",
	AuthTypes: []provider.AuthType{},
	Factory: func(opts map[string]any) (models.Model, error) {
		return New(Config{Options: opts})
	},
}

func init() { provider.Register(Meta) }

// Config controls construction of a Client.
type Config struct {
	BaseURL string
	Model   string
	Options map[string]any
}

const (
	defaultBaseURL = "http://localhost:11434"
	httpTimeout    = 10 * time.Minute
)

// Client is a configured Ollama Model. One Client = one model id.
type Client struct {
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// New constructs a Client. Model name comes from Config.Model or
// Options["model"]. Returns an error if neither is set.
func New(cfg Config) (*Client, error) {
	model := cfg.Model
	if model == "" {
		if m, ok := cfg.Options["model"].(string); ok {
			model = m
		}
	}
	if model == "" {
		return nil, fmt.Errorf("ollama: model is required (set Config.Model or Options[\"model\"])")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if u, ok := cfg.Options["base_url"].(string); ok && u != "" {
			baseURL = u
		}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	var maxTokens int
	if v, ok := cfg.Options["max_tokens"]; ok {
		switch n := v.(type) {
		case int:
			maxTokens = n
		case float64:
			maxTokens = int(n)
		}
	}

	return &Client{
		baseURL:    baseURL,
		model:      model,
		maxTokens:  maxTokens,
		httpClient: &http.Client{Timeout: httpTimeout},
	}, nil
}

// Info returns catalog ModelInfo for this Ollama model. Falls back to a bare
// ModelInfo if the model id isn't in the catalog (common for custom models).
func (c *Client) Info() models.ModelInfo {
	if info, ok := catalog.Get("ollama", c.model); ok {
		return info
	}
	return models.ModelInfo{Provider: "ollama", ID: c.model}
}

// CountTokens uses a chars/4 heuristic. Ollama exposes no tokenizer endpoint,
// and shelling out to a tokenizer per request would dominate latency. The
// heuristic is conservative: it generally over-estimates English text by
// 10-30%, which is the safe bias for compaction (trigger sooner, not later).
//
// If a future Ollama release exposes /api/tokenize we'll switch to that and
// keep this as a fallback.
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
				if b, err := json.Marshal(v.Input); err == nil {
					total += len(v.Name) + len(b)
				}
			case models.ToolResultPart:
				for _, out := range v.Output {
					if t, ok := out.(models.TextPart); ok {
						total += len(t.Text)
					}
				}
			}
		}
	}
	return total / 4, nil
}

// origin returns the MessageOrigin stamped on every assistant message this
// client produces. Note: API is intentionally left empty. Per
// models.API's contract, API constants are reserved for wire formats
// shared across multiple providers (e.g. APIOpenAICompletions). Ollama's
// /api/chat is provider-specific. The empty API does mean
// MessageOrigin.SameModel will report false even for Ollama→Ollama replays
// (Target.origin() returns nil when API is empty), so the transform layer
// will always treat Ollama messages as cross-model. That is conservative
// but correct: it drops reasoning blocks on replay, and Ollama models in
// our catalog don't emit signed reasoning that would benefit from the
// fast path anyway.
func (c *Client) origin() *models.MessageOrigin {
	return &models.MessageOrigin{
		Provider: "ollama",
		ModelID:  c.model,
	}
}

// Stream begins a streaming /api/chat request. req.Messages is normalized
// via transform.Apply for this target before serialization.
func (c *Client) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	info := c.Info()
	req.Messages = transform.Apply(req.Messages, transform.Target{
		Provider:     "ollama",
		ModelID:      c.model,
		Capabilities: info.Capabilities,
		// API intentionally omitted; see origin() for rationale.
	})

	wireReq := c.buildRequest(req)
	wireReq.Stream = true

	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: API %d: %s", resp.StatusCode, string(raw))
	}

	out := models.NewEventStream[models.StreamPart, *models.Message](64)
	go runStream(ctx, resp, out, c.origin())
	return out, nil
}

// runStream reads JSON-line responses from Ollama and emits StreamParts.
// origin is stamped on the assembled assistant message.
func runStream(ctx context.Context, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	p := newStreamParser(out, origin)
	p.start()

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			p.terminateAborted()
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if done := p.handle([]byte(line)); done {
			return
		}
	}

	if err := scanner.Err(); err != nil {
		p.terminateError(fmt.Errorf("ollama: scanner: %w", err))
		return
	}
	p.terminateError(fmt.Errorf("ollama: stream closed without done=true"))
}

// streamParser assembles Ollama's chat stream. Unlike Anthropic's per-block
// SSE, Ollama emits one chunk per line containing a partial assistant
// message; we maintain a single text block id and emit deltas as content
// arrives.
type streamParser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	terminated bool

	// origin is stamped on the assembled assistant message so downstream
	// transform.Apply calls can detect same-model replay.
	origin *models.MessageOrigin

	textID       string // current open text block id; "" if none
	textBuf      strings.Builder
	content      models.Content
	usage        models.Usage
	doneReason   string
	createdAt    string
	hasToolCalls bool
}

func newStreamParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin) *streamParser {
	return &streamParser{out: out, origin: origin}
}

func (p *streamParser) start() {
	p.out.Push(models.StreamStartPart{})
}

// handle parses one JSON line from /api/chat. Returns true when terminal.
func (p *streamParser) handle(line []byte) bool {
	var ev struct {
		Model     string `json:"model"`
		CreatedAt string `json:"created_at"`
		Message   struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Thinking  string `json:"thinking,omitempty"`
			ToolCalls []struct {
				Function struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		Done            bool   `json:"done"`
		DoneReason      string `json:"done_reason"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}
	if err := json.Unmarshal(line, &ev); err != nil {
		p.terminateError(fmt.Errorf("ollama: parse chunk: %w", err))
		return true
	}

	if p.createdAt == "" && ev.CreatedAt != "" {
		p.createdAt = ev.CreatedAt
		p.out.Push(models.ResponseMetadataPart{
			ResponseMetadata: models.ResponseMetadata{
				ID:      ev.CreatedAt,
				ModelID: ev.Model,
			},
		})
	}

	// Stream text deltas as they arrive. Open the text block lazily on the
	// first non-empty content.
	if ev.Message.Content != "" {
		if p.textID == "" {
			p.textID = "txt_0"
			p.out.Push(models.TextStartPart{ID: p.textID})
		}
		p.textBuf.WriteString(ev.Message.Content)
		p.out.Push(models.TextDeltaPart{ID: p.textID, Delta: ev.Message.Content})
	}

	if !ev.Done {
		return false
	}

	// Terminal frame. Close any open text block.
	if p.textID != "" {
		p.content = append(p.content, models.TextPart{Text: p.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: p.textID})
	}

	// Tool calls land on the final frame in Ollama's protocol. Synthesize
	// the three-phase flow per call so consumers see a uniform shape across
	// providers.
	for i, tc := range ev.Message.ToolCalls {
		p.hasToolCalls = true
		callID := "call_" + strconv.Itoa(i)
		p.out.Push(models.ToolInputStartPart{ID: callID, ToolName: tc.Function.Name})
		if raw, err := json.Marshal(tc.Function.Arguments); err == nil {
			p.out.Push(models.ToolInputDeltaPart{ID: callID, Delta: string(raw)})
		}
		p.out.Push(models.ToolInputEndPart{ID: callID})
		p.out.Push(models.ToolCallPart_{
			ID:       callID,
			ToolName: tc.Function.Name,
			Input:    tc.Function.Arguments,
		})
		p.content = append(p.content, models.ToolCallPart{
			CallID: callID,
			Name:   tc.Function.Name,
			Input:  tc.Function.Arguments,
		})
	}

	p.usage = models.Usage{
		InputTokens:  ev.PromptEvalCount,
		OutputTokens: ev.EvalCount,
		TotalTokens:  ev.PromptEvalCount + ev.EvalCount,
	}
	p.doneReason = ev.DoneReason

	p.terminateNormal()
	return true
}

func (p *streamParser) terminateNormal() {
	reason := p.finishReason()
	msg := &models.Message{
		Role:         models.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: reason,
	}
	p.out.Push(models.FinishPart{
		Reason:  reason,
		Usage:   p.usage,
		Message: msg,
		Metadata: models.ResponseMetadata{
			ID:      p.createdAt,
			ModelID: "",
		},
	})
	p.close(msg, nil)
}

func (p *streamParser) terminateError(err error) {
	if p.terminated {
		return
	}
	p.out.Push(models.ErrorPart{Message: err.Error()})
	msg := &models.Message{
		Role:         models.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: models.FinishReasonError,
	}
	p.out.Push(models.FinishPart{
		Reason:  models.FinishReasonError,
		Usage:   p.usage,
		Message: msg,
	})
	p.close(msg, err)
}

func (p *streamParser) terminateAborted() {
	if p.terminated {
		return
	}
	msg := &models.Message{
		Role:         models.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: models.FinishReasonAborted,
	}
	p.out.Push(models.FinishPart{
		Reason:  models.FinishReasonAborted,
		Usage:   p.usage,
		Message: msg,
	})
	p.close(msg, context.Canceled)
}

func (p *streamParser) close(msg *models.Message, err error) {
	p.terminated = true
	p.out.Close(msg, err)
}

// finishReason normalizes Ollama's done_reason. When the model emitted any
// tool calls we override to tool-calls regardless of done_reason, since
// Ollama reports "stop" even when tools were called.
func (p *streamParser) finishReason() models.FinishReason {
	if p.hasToolCalls {
		return models.FinishReasonToolCalls
	}
	switch p.doneReason {
	case "stop", "":
		return models.FinishReasonStop
	case "length":
		return models.FinishReasonLength
	case "load":
		// Model failed to load; treat as error.
		return models.FinishReasonError
	default:
		return models.FinishReasonOther
	}
}

// ---- request building ------------------------------------------------------

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	Function toolCallFunction `json:"function"`
}

type toolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type modelOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type request struct {
	Model      string           `json:"model"`
	Messages   []chatMessage    `json:"messages"`
	Tools      []toolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
	Format     map[string]any   `json:"format,omitempty"`
	Options    modelOptions     `json:"options"`
	Stream     bool             `json:"stream"`
}

func (c *Client) buildRequest(req models.Request) request {
	var messages []chatMessage
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, c.toWireMessages(m)...)
	}

	var tools []toolDefinition
	for _, t := range req.Tools {
		tools = append(tools, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = c.maxTokens
	}

	// Map ToolChoice to Ollama's string field.
	// Ollama supports "auto" (default) and "none". "required" maps to "any"
	// in Ollama's newer builds; fall back to "required" for older builds.
	// Specific tool forcing is not supported by Ollama's API; we send "auto".
	var toolChoice string
	if len(tools) > 0 {
		switch req.ToolChoice.Mode {
		case models.ToolChoiceNone:
			toolChoice = "none"
		case models.ToolChoiceRequired:
			toolChoice = "required"
		// ToolChoiceAuto, ToolChoiceTool, and zero value: omit (Ollama defaults to auto).
		}
	}

	return request{
		Model:      c.model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: toolChoice,
		Format:     outputFormat(req.OutputSchema),
		Options:    modelOptions{NumPredict: maxOut},
	}
}

// outputFormat returns the Ollama format= payload for a models.OutputSchema.
// Ollama's /api/chat accepts the raw JSON schema object as the format field.
func outputFormat(s *models.OutputSchema) map[string]any {
	if s == nil {
		return nil
	}
	return s.Schema
}

// toWireMessages converts a models.Message into one or more Ollama chat
// messages. Ollama uses one message per logical entry: text content goes in
// {role,content}; tool calls go in {role:"assistant", tool_calls}; tool
// results go in {role:"tool", tool_call_id, content}.
func (c *Client) toWireMessages(m models.Message) []chatMessage {
	var out []chatMessage
	for _, p := range m.Content {
		switch v := p.(type) {
		case models.TextPart:
			if v.Text == "" {
				continue
			}
			out = append(out, chatMessage{Role: string(m.Role), Content: v.Text})
		case models.ReasoningPart:
			// Ollama doesn't accept thinking on input round-trip; drop.
		case models.ToolCallPart:
			out = append(out, chatMessage{
				Role:      "assistant",
				ToolCalls: []toolCall{{Function: toolCallFunction{Name: v.Name, Arguments: v.Input}}},
			})
		case models.ToolResultPart:
			// Flatten Output to text. Ollama doesn't accept structured tool
			// results.
			var sb strings.Builder
			for _, out := range v.Output {
				if t, ok := out.(models.TextPart); ok {
					sb.WriteString(t.Text)
				}
			}
			out = append(out, chatMessage{
				Role:       "tool",
				ToolCallID: v.CallID,
				Content:    sb.String(),
			})
		}
	}
	return out
}

// Compile-time check that *Client satisfies models.Model.
var _ models.Model = (*Client)(nil)
