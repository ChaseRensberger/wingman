package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"wingman/models"
	"wingman/provider"
)

const (
	defaultModel       = "claude-sonnet-4-20250514"
	defaultMaxTokens   = 8192
	defaultTemperature = 1.0
	apiURL             = "https://api.anthropic.com/v1/messages"
	apiVersion         = "2023-06-01"
	httpTimeout        = 5 * time.Minute
)

type Config struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature *float64
}

type Client struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

func New(cfg Config) *Client {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := cfg.Model
	if model == "" {
		model = defaultModel
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
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}
}

type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type toolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"input_schema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type request struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []toolDefinition   `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type response struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []contentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        usage          `json:"usage"`
}

type streamEvent struct {
	Type         string        `json:"type"`
	Message      *response     `json:"message,omitempty"`
	Index        int           `json:"index,omitempty"`
	ContentBlock *contentBlock `json:"content_block,omitempty"`
	Delta        *streamDelta  `json:"delta,omitempty"`
	Usage        *usage        `json:"usage,omitempty"`
	Error        *streamError  `json:"error,omitempty"`
}

type streamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type streamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (c *Client) RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error) {
	anthropicReq := c.buildRequest(req, false)

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

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

func (c *Client) StreamInference(ctx context.Context, req models.WingmanInferenceRequest) (<-chan provider.StreamEvent, error) {
	anthropicReq := c.buildRequest(req, true)

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	events := make(chan provider.StreamEvent, 100)

	go c.processStream(ctx, resp.Body, events)

	return events, nil
}

func (c *Client) processStream(ctx context.Context, body io.ReadCloser, events chan<- provider.StreamEvent) {
	defer close(events)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var currentEvent string
	var contentBlocks []contentBlock
	var toolInputBuffers = make(map[int]string)
	var finalUsage models.WingmanUsage
	var stopReason string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- provider.StreamEvent{
				Type:  provider.StreamEventError,
				Error: ctx.Err(),
			}
			return
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			var event streamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch currentEvent {
			case "message_start":
				if event.Message != nil && event.Message.Usage.InputTokens > 0 {
					finalUsage.InputTokens = event.Message.Usage.InputTokens
				}

			case "content_block_start":
				for len(contentBlocks) <= event.Index {
					contentBlocks = append(contentBlocks, contentBlock{})
				}
				if event.ContentBlock != nil {
					contentBlocks[event.Index] = *event.ContentBlock
				}

			case "content_block_delta":
				if event.Delta != nil {
					switch event.Delta.Type {
					case "text_delta":
						if event.Index < len(contentBlocks) {
							contentBlocks[event.Index].Text += event.Delta.Text
						}
						events <- provider.StreamEvent{
							Type:    provider.StreamEventToken,
							Content: event.Delta.Text,
						}
					case "input_json_delta":
						toolInputBuffers[event.Index] += event.Delta.PartialJSON
					}
				}

			case "content_block_stop":
				if event.Index < len(contentBlocks) {
					block := &contentBlocks[event.Index]
					if block.Type == "tool_use" {
						if jsonStr, ok := toolInputBuffers[event.Index]; ok {
							var input any
							json.Unmarshal([]byte(jsonStr), &input)
							block.Input = input
							delete(toolInputBuffers, event.Index)
						}
						events <- provider.StreamEvent{
							Type: provider.StreamEventToolCall,
							Delta: models.WingmanContentBlock{
								Type:  models.ContentTypeToolUse,
								ID:    block.ID,
								Name:  block.Name,
								Input: block.Input,
							},
						}
					}
				}

			case "message_delta":
				if event.Delta != nil && event.Delta.StopReason != "" {
					stopReason = event.Delta.StopReason
				}
				if event.Usage != nil {
					finalUsage.OutputTokens = event.Usage.OutputTokens
				}

			case "message_stop":
				events <- provider.StreamEvent{
					Type:  provider.StreamEventUsage,
					Usage: &finalUsage,
				}
				events <- provider.StreamEvent{
					Type: provider.StreamEventDone,
					Delta: &models.WingmanInferenceResponse{
						Content:    c.toWingmanContentBlocks(contentBlocks),
						StopReason: stopReason,
						Usage:      finalUsage,
					},
				}

			case "error":
				if event.Error != nil {
					events <- provider.StreamEvent{
						Type:  provider.StreamEventError,
						Error: fmt.Errorf("%s: %s", event.Error.Type, event.Error.Message),
					}
				}

			case "ping":
				// Heartbeat, ignore
			}
		}
	}

	if err := scanner.Err(); err != nil {
		events <- provider.StreamEvent{
			Type:  provider.StreamEventError,
			Error: fmt.Errorf("stream read error: %w", err),
		}
	}
}

func (c *Client) toWingmanContentBlocks(blocks []contentBlock) []models.WingmanContentBlock {
	result := make([]models.WingmanContentBlock, len(blocks))
	for i, b := range blocks {
		result[i] = models.WingmanContentBlock{
			Type:      models.WingmanContentType(b.Type),
			Text:      b.Text,
			ID:        b.ID,
			Name:      b.Name,
			Input:     b.Input,
			ToolUseID: b.ToolUseID,
			Content:   b.Content,
			IsError:   b.IsError,
		}
	}
	return result
}

func (c *Client) buildRequest(req models.WingmanInferenceRequest, stream bool) request {
	messages := make([]anthropicMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = c.toAnthropicMessage(msg)
	}

	var tools []toolDefinition
	if len(req.Tools) > 0 {
		tools = make([]toolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = c.toAnthropicTool(t)
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}

	temperature := c.temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	return request{
		Model:       c.model,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      req.Instructions,
		Messages:    messages,
		Tools:       tools,
		Stream:      stream,
	}
}

func (c *Client) toAnthropicMessage(msg models.WingmanMessage) anthropicMessage {
	blocks := make([]contentBlock, len(msg.Content))
	for i, b := range msg.Content {
		blocks[i] = contentBlock{
			Type:      string(b.Type),
			Text:      b.Text,
			ID:        b.ID,
			Name:      b.Name,
			Input:     b.Input,
			ToolUseID: b.ToolUseID,
			Content:   b.Content,
			IsError:   b.IsError,
		}
	}
	return anthropicMessage{
		Role:    string(msg.Role),
		Content: blocks,
	}
}

func (c *Client) toAnthropicTool(t models.WingmanToolDefinition) toolDefinition {
	props := make(map[string]property)
	for name, p := range t.InputSchema.Properties {
		props[name] = property{
			Type:        p.Type,
			Description: p.Description,
			Enum:        p.Enum,
		}
	}
	return toolDefinition{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: inputSchema{
			Type:       t.InputSchema.Type,
			Properties: props,
			Required:   t.InputSchema.Required,
		},
	}
}

func (c *Client) toWingmanResponse(resp response) *models.WingmanInferenceResponse {
	return &models.WingmanInferenceResponse{
		ID:         resp.ID,
		Content:    c.toWingmanContentBlocks(resp.Content),
		StopReason: resp.StopReason,
		Usage: models.WingmanUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}
}
