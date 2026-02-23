package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/provider"
)

// ============================================================
//  Registry registration
// ============================================================

// Meta is the provider metadata registered in the default provider registry.
var Meta = provider.ProviderMeta{
	ID:        "ollama",
	Name:      "Ollama",
	AuthTypes: []provider.AuthType{},
	Factory: func(opts map[string]any) (core.Provider, error) {
		return New(Config{Options: opts})
	},
}

func init() {
	provider.Register(Meta)
}

// ============================================================
//  Config and constructor
// ============================================================

// Config configures the Ollama provider.
//
// BaseURL is optional; defaults to http://localhost:11434. It can also be set
// via Options["base_url"].
//
// Options recognises the following keys:
//
//   - "model"      string      — model name (REQUIRED; returns error if absent)
//   - "base_url"   string      — alternative to the BaseURL field
//   - "max_tokens" int/float64 — maps to Ollama's num_predict
//   - "temperature" float64    — sampling temperature
type Config struct {
	BaseURL string         // optional; defaults to http://localhost:11434
	Options map[string]any // model (required), max_tokens, temperature, base_url
}

// Client implements core.Provider for the Ollama chat API.
type Client struct {
	baseURL     string
	model       string
	maxTokens   int
	temperature *float64
	httpClient  *http.Client
}

const (
	defaultBaseURL = "http://localhost:11434"
	httpTimeout    = 10 * time.Minute
)

// New creates an Ollama Client. Returns an error if no model is specified.
func New(cfg Config) (*Client, error) {
	model, _ := cfg.Options["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("ollama: Options[\"model\"] is required")
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

	var temperature *float64
	if v, ok := cfg.Options["temperature"]; ok {
		if f, ok := v.(float64); ok {
			temperature = &f
		}
	}

	return &Client{
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}, nil
}

// ============================================================
//  Internal wire types
// ============================================================

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
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
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
}

type request struct {
	Model    string           `json:"model"`
	Messages []chatMessage    `json:"messages"`
	Tools    []toolDefinition `json:"tools,omitempty"`
	Format   any              `json:"format,omitempty"`
	Options  modelOptions     `json:"options"`
	Stream   bool             `json:"stream"`
}

type response struct {
	Model              string      `json:"model"`
	CreatedAt          string      `json:"created_at"`
	Message            chatMessage `json:"message"`
	Done               bool        `json:"done"`
	DoneReason         string      `json:"done_reason"`
	TotalDuration      int64       `json:"total_duration"`
	LoadDuration       int64       `json:"load_duration"`
	PromptEvalCount    int         `json:"prompt_eval_count"`
	PromptEvalDuration int64       `json:"prompt_eval_duration"`
	EvalCount          int         `json:"eval_count"`
	EvalDuration       int64       `json:"eval_duration"`
}

// ============================================================
//  Type conversions
// ============================================================

func (c *Client) toOllamaMessages(msg core.Message) []chatMessage {
	var result []chatMessage

	for _, block := range msg.Content {
		switch block.Type {
		case core.ContentTypeText:
			result = append(result, chatMessage{
				Role:    string(msg.Role),
				Content: block.Text,
			})

		case core.ContentTypeToolUse:
			args, _ := block.Input.(map[string]any)
			result = append(result, chatMessage{
				Role: "assistant",
				ToolCalls: []toolCall{{
					Function: toolCallFunction{
						Name:      block.Name,
						Arguments: args,
					},
				}},
			})

		case core.ContentTypeToolResult:
			result = append(result, chatMessage{
				Role:       "tool",
				Content:    block.Content,
				ToolCallID: block.ToolUseID,
			})
		}
	}

	return result
}

func (c *Client) toOllamaTool(t core.ToolDefinition) toolDefinition {
	props := make(map[string]any)
	for name, p := range t.InputSchema.Properties {
		prop := map[string]any{"type": p.Type}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		props[name] = prop
	}

	return toolDefinition{
		Type: "function",
		Function: toolFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters: map[string]any{
				"type":       t.InputSchema.Type,
				"properties": props,
				"required":   t.InputSchema.Required,
			},
		},
	}
}

func (c *Client) buildRequest(req core.InferenceRequest) request {
	var messages []chatMessage

	if req.Instructions != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.Instructions})
	}

	for _, msg := range req.Messages {
		messages = append(messages, c.toOllamaMessages(msg)...)
	}

	var tools []toolDefinition
	if len(req.Tools) > 0 {
		tools = make([]toolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = c.toOllamaTool(t)
		}
	}

	r := request{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Options: modelOptions{
			Temperature: c.temperature,
			NumPredict:  c.maxTokens,
		},
	}

	if req.OutputSchema != nil {
		r.Format = req.OutputSchema
	}

	return r
}

func (c *Client) toInferenceResponse(resp response) *core.InferenceResponse {
	var content []core.ContentBlock

	if resp.Message.Content != "" {
		content = append(content, core.ContentBlock{
			Type: core.ContentTypeText,
			Text: resp.Message.Content,
		})
	}

	for i, tc := range resp.Message.ToolCalls {
		content = append(content, core.ContentBlock{
			Type:  core.ContentTypeToolUse,
			ID:    fmt.Sprintf("tool_%d", i),
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	stopReason := resp.DoneReason
	if len(resp.Message.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	return &core.InferenceResponse{
		ID:         resp.CreatedAt,
		Content:    content,
		StopReason: stopReason,
		Usage: core.Usage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
	}
}

// ============================================================
//  Provider interface implementation
// ============================================================

// RunInference performs a blocking inference call.
func (c *Client) RunInference(ctx context.Context, req core.InferenceRequest) (*core.InferenceResponse, error) {
	ollamaReq := c.buildRequest(req)
	ollamaReq.Stream = false

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("ollama: failed to parse response: %w", err)
	}

	return c.toInferenceResponse(apiResp), nil
}

// StreamInference begins a streaming inference call.
func (c *Client) StreamInference(ctx context.Context, req core.InferenceRequest) (core.Stream, error) {
	ollamaReq := c.buildRequest(req)
	ollamaReq.Stream = true

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(body))
	}

	return newStream(resp), nil
}
