// Package ollama implements wingmodels.Model for a local Ollama server.
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

	"github.com/chaserensberger/wingman/wingmodels"
	"github.com/chaserensberger/wingman/wingmodels/catalog"
	provider "github.com/chaserensberger/wingman/wingmodels/providers"
)

// Meta is the registry entry for the Ollama provider. No auth (Ollama runs
// locally by default).
var Meta = provider.ProviderMeta{
	ID:        "ollama",
	Name:      "Ollama",
	AuthTypes: []provider.AuthType{},
	Factory: func(opts map[string]any) (wingmodels.Model, error) {
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
func (c *Client) Info() wingmodels.ModelInfo {
	if info, ok := catalog.Get("ollama", c.model); ok {
		return info
	}
	return wingmodels.ModelInfo{Provider: "ollama", ID: c.model}
}

// CountTokens uses a chars/4 heuristic. Ollama exposes no tokenizer endpoint,
// and shelling out to a tokenizer per request would dominate latency. The
// heuristic is conservative: it generally over-estimates English text by
// 10-30%, which is the safe bias for compaction (trigger sooner, not later).
//
// If a future Ollama release exposes /api/tokenize we'll switch to that and
// keep this as a fallback.
func (c *Client) CountTokens(_ context.Context, msgs []wingmodels.Message) (int, error) {
	total := 0
	for _, m := range msgs {
		for _, p := range m.Content {
			switch v := p.(type) {
			case wingmodels.TextPart:
				total += len(v.Text)
			case wingmodels.ReasoningPart:
				total += len(v.Reasoning)
			case wingmodels.ToolCallPart:
				if b, err := json.Marshal(v.Input); err == nil {
					total += len(v.Name) + len(b)
				}
			case wingmodels.ToolResultPart:
				for _, out := range v.Output {
					if t, ok := out.(wingmodels.TextPart); ok {
						total += len(t.Text)
					}
				}
			}
		}
	}
	return total / 4, nil
}

// Stream begins a streaming /api/chat request.
func (c *Client) Stream(ctx context.Context, req wingmodels.Request) (*wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], error) {
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

	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](64)
	go runStream(ctx, resp, out)
	return out, nil
}

// runStream reads JSON-line responses from Ollama and emits StreamParts.
func runStream(ctx context.Context, resp *http.Response, out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	p := newStreamParser(out)
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
	out        *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]
	terminated bool

	textID       string // current open text block id; "" if none
	textBuf      strings.Builder
	content      wingmodels.Content
	usage        wingmodels.Usage
	doneReason   string
	createdAt    string
	hasToolCalls bool
}

func newStreamParser(out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) *streamParser {
	return &streamParser{out: out}
}

func (p *streamParser) start() {
	p.out.Push(wingmodels.StreamStartPart{})
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
		p.out.Push(wingmodels.ResponseMetadataPart{
			ResponseMetadata: wingmodels.ResponseMetadata{
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
			p.out.Push(wingmodels.TextStartPart{ID: p.textID})
		}
		p.textBuf.WriteString(ev.Message.Content)
		p.out.Push(wingmodels.TextDeltaPart{ID: p.textID, Delta: ev.Message.Content})
	}

	if !ev.Done {
		return false
	}

	// Terminal frame. Close any open text block.
	if p.textID != "" {
		p.content = append(p.content, wingmodels.TextPart{Text: p.textBuf.String()})
		p.out.Push(wingmodels.TextEndPart{ID: p.textID})
	}

	// Tool calls land on the final frame in Ollama's protocol. Synthesize
	// the three-phase flow per call so consumers see a uniform shape across
	// providers.
	for i, tc := range ev.Message.ToolCalls {
		p.hasToolCalls = true
		callID := "call_" + strconv.Itoa(i)
		p.out.Push(wingmodels.ToolInputStartPart{ID: callID, ToolName: tc.Function.Name})
		if raw, err := json.Marshal(tc.Function.Arguments); err == nil {
			p.out.Push(wingmodels.ToolInputDeltaPart{ID: callID, Delta: string(raw)})
		}
		p.out.Push(wingmodels.ToolInputEndPart{ID: callID})
		p.out.Push(wingmodels.ToolCallPart_{
			ID:       callID,
			ToolName: tc.Function.Name,
			Input:    tc.Function.Arguments,
		})
		p.content = append(p.content, wingmodels.ToolCallPart{
			CallID: callID,
			Name:   tc.Function.Name,
			Input:  tc.Function.Arguments,
		})
	}

	p.usage = wingmodels.Usage{
		InputTokens:  ev.PromptEvalCount,
		OutputTokens: ev.EvalCount,
		TotalTokens:  ev.PromptEvalCount + ev.EvalCount,
	}
	p.doneReason = ev.DoneReason

	p.terminateNormal()
	return true
}

func (p *streamParser) terminateNormal() {
	msg := &wingmodels.Message{Role: wingmodels.RoleAssistant, Content: p.content}
	p.out.Push(wingmodels.FinishPart{
		Reason:  p.finishReason(),
		Usage:   p.usage,
		Message: msg,
		Metadata: wingmodels.ResponseMetadata{
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
	p.out.Push(wingmodels.ErrorPart{Message: err.Error()})
	msg := &wingmodels.Message{Role: wingmodels.RoleAssistant, Content: p.content}
	p.out.Push(wingmodels.FinishPart{
		Reason:  wingmodels.FinishReasonError,
		Usage:   p.usage,
		Message: msg,
	})
	p.close(msg, err)
}

func (p *streamParser) terminateAborted() {
	if p.terminated {
		return
	}
	msg := &wingmodels.Message{Role: wingmodels.RoleAssistant, Content: p.content}
	p.out.Push(wingmodels.FinishPart{
		Reason:  wingmodels.FinishReasonAborted,
		Usage:   p.usage,
		Message: msg,
	})
	p.close(msg, context.Canceled)
}

func (p *streamParser) close(msg *wingmodels.Message, err error) {
	p.terminated = true
	p.out.Close(msg, err)
}

// finishReason normalizes Ollama's done_reason. When the model emitted any
// tool calls we override to tool-calls regardless of done_reason, since
// Ollama reports "stop" even when tools were called.
func (p *streamParser) finishReason() wingmodels.FinishReason {
	if p.hasToolCalls {
		return wingmodels.FinishReasonToolCalls
	}
	switch p.doneReason {
	case "stop", "":
		return wingmodels.FinishReasonStop
	case "length":
		return wingmodels.FinishReasonLength
	case "load":
		// Model failed to load; treat as error.
		return wingmodels.FinishReasonError
	default:
		return wingmodels.FinishReasonOther
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
	Model    string           `json:"model"`
	Messages []chatMessage    `json:"messages"`
	Tools    []toolDefinition `json:"tools,omitempty"`
	Options  modelOptions     `json:"options"`
	Stream   bool             `json:"stream"`
}

func (c *Client) buildRequest(req wingmodels.Request) request {
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
	return request{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Options:  modelOptions{NumPredict: maxOut},
	}
}

// toWireMessages converts a wingmodels.Message into one or more Ollama chat
// messages. Ollama uses one message per logical entry: text content goes in
// {role,content}; tool calls go in {role:"assistant", tool_calls}; tool
// results go in {role:"tool", tool_call_id, content}.
func (c *Client) toWireMessages(m wingmodels.Message) []chatMessage {
	var out []chatMessage
	for _, p := range m.Content {
		switch v := p.(type) {
		case wingmodels.TextPart:
			if v.Text == "" {
				continue
			}
			out = append(out, chatMessage{Role: string(m.Role), Content: v.Text})
		case wingmodels.ReasoningPart:
			// Ollama doesn't accept thinking on input round-trip; drop.
		case wingmodels.ToolCallPart:
			out = append(out, chatMessage{
				Role:      "assistant",
				ToolCalls: []toolCall{{Function: toolCallFunction{Name: v.Name, Arguments: v.Input}}},
			})
		case wingmodels.ToolResultPart:
			// Flatten Output to text. Ollama doesn't accept structured tool
			// results.
			var sb strings.Builder
			for _, out := range v.Output {
				if t, ok := out.(wingmodels.TextPart); ok {
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

// Compile-time check that *Client satisfies wingmodels.Model.
var _ wingmodels.Model = (*Client)(nil)
