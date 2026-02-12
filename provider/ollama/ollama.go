package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"wingman/models"
	"wingman/provider"
)

var Meta = provider.ProviderMeta{
	Name:        "ollama",
	DisplayName: "Ollama",
	AuthTypes:   []provider.AuthType{},
}

func init() {
	provider.Register(Meta)
}

type Config struct {
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature *float64
}

type Client struct {
	baseURL     string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

const (
	defaultBaseURL     = "http://localhost:11434"
	defaultMaxTokens   = 8192
	defaultTemperature = 0.7
	httpTimeout        = 10 * time.Minute
)

func New(cfg Config) *Client {
	if cfg.Model == "" {
		return nil
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	temperature := defaultTemperature
	if cfg.Temperature != nil {
		temperature = *cfg.Temperature
	}

	return &Client{
		baseURL:     baseURL,
		model:       cfg.Model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}
}

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
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
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

func (c *Client) RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error) {
	ollamaReq := c.buildRequest(req)
	ollamaReq.Stream = false

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return c.toWingmanResponse(apiResp), nil
}

func (c *Client) buildRequest(req models.WingmanInferenceRequest) request {
	var messages []chatMessage

	if req.Instructions != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
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

func (c *Client) toOllamaMessages(msg models.WingmanMessage) []chatMessage {
	var result []chatMessage

	for _, block := range msg.Content {
		switch block.Type {
		case models.ContentTypeText:
			result = append(result, chatMessage{
				Role:    string(msg.Role),
				Content: block.Text,
			})

		case models.ContentTypeToolUse:
			args, _ := block.Input.(map[string]any)
			result = append(result, chatMessage{
				Role: "assistant",
				ToolCalls: []toolCall{
					{
						Function: toolCallFunction{
							Name:      block.Name,
							Arguments: args,
						},
					},
				},
			})

		case models.ContentTypeToolResult:
			result = append(result, chatMessage{
				Role:       "tool",
				Content:    block.Content,
				ToolCallID: block.ToolUseID,
			})
		}
	}

	return result
}

func (c *Client) toOllamaTool(t models.WingmanToolDefinition) toolDefinition {
	props := make(map[string]any)
	for name, p := range t.InputSchema.Properties {
		prop := map[string]any{
			"type": p.Type,
		}
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

func (c *Client) toWingmanResponse(resp response) *models.WingmanInferenceResponse {
	var content []models.WingmanContentBlock

	if resp.Message.Content != "" {
		content = append(content, models.WingmanContentBlock{
			Type: models.ContentTypeText,
			Text: resp.Message.Content,
		})
	}

	for i, tc := range resp.Message.ToolCalls {
		content = append(content, models.WingmanContentBlock{
			Type:  models.ContentTypeToolUse,
			ID:    fmt.Sprintf("tool_%d", i),
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	stopReason := resp.DoneReason
	if len(resp.Message.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	return &models.WingmanInferenceResponse{
		ID:         resp.CreatedAt,
		Content:    content,
		StopReason: stopReason,
		Usage: models.WingmanUsage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
	}
}

func (c *Client) StreamInference(ctx context.Context, req models.WingmanInferenceRequest) (provider.Stream, error) {
	ollamaReq := c.buildRequest(req)
	ollamaReq.Stream = true

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return newStream(resp), nil
}
