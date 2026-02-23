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

	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/provider"
)

// ============================================================
//  Registry registration
// ============================================================

// Meta is the provider metadata registered in the default provider registry.
var Meta = provider.ProviderMeta{
	ID:        "anthropic",
	Name:      "Anthropic",
	AuthTypes: []provider.AuthType{provider.AuthTypeAPIKey},
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

// Config configures the Anthropic provider.
//
// APIKey is optional; if empty the provider falls back to Options["api_key"]
// and then the ANTHROPIC_API_KEY environment variable.
//
// Options recognises the following keys:
//
//   - "model"       string   — model ID (default: "claude-sonnet-4-5")
//   - "max_tokens"  int/float64 — maximum output tokens (default: 4096)
//   - "temperature" float64  — sampling temperature (omitted if not set)
//   - "api_key"     string   — alternative to the APIKey field
type Config struct {
	APIKey  string         // optional; falls back to Options["api_key"] then env
	Options map[string]any // inference parameters and optional auth override
}

// Client implements core.Provider for the Anthropic Messages API.
type Client struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature *float64
	httpClient  *http.Client
}

const (
	defaultModel     = "claude-sonnet-4-5"
	defaultMaxTokens = 4096
	apiURL           = "https://api.anthropic.com/v1/messages"
	apiVersion       = "2023-06-01"
	httpTimeout      = 5 * time.Minute
)

// New creates an Anthropic Client. It returns an error if no API key can be
// resolved (Config.APIKey, Options["api_key"], or ANTHROPIC_API_KEY env var).
func New(cfg ...Config) (*Client, error) {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}

	apiKey := c.APIKey
	if apiKey == "" {
		if k, ok := c.Options["api_key"].(string); ok && k != "" {
			apiKey = k
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: no API key provided (set Config.APIKey, Options[\"api_key\"], or ANTHROPIC_API_KEY)")
	}

	model := defaultModel
	if m, ok := c.Options["model"].(string); ok && m != "" {
		model = m
	}

	// Default max_tokens to 4096 — Anthropic's API requires this field and
	// returns a 400 if it is absent. Users can override via Options["max_tokens"].
	maxTokens := defaultMaxTokens
	if v, ok := c.Options["max_tokens"]; ok {
		switch n := v.(type) {
		case int:
			maxTokens = n
		case float64:
			maxTokens = int(n)
		}
	}

	var temperature *float64
	if v, ok := c.Options["temperature"]; ok {
		if f, ok := v.(float64); ok {
			temperature = &f
		}
	}

	return &Client{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}, nil
}

// ============================================================
//  Internal wire types
// ============================================================

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

type outputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema,omitempty"`
}

type outputConfig struct {
	Format *outputFormat `json:"format,omitempty"`
}

type request struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	Temperature  *float64           `json:"temperature,omitempty"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	Tools        []toolDefinition   `json:"tools,omitempty"`
	OutputConfig *outputConfig      `json:"output_config,omitempty"`
	Stream       bool               `json:"stream,omitempty"`
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

// ============================================================
//  Type conversions
// ============================================================

func (c *Client) toAnthropicMessage(msg core.Message) anthropicMessage {
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
	return anthropicMessage{Role: string(msg.Role), Content: blocks}
}

func (c *Client) toAnthropicTool(t core.ToolDefinition) toolDefinition {
	props := make(map[string]property)
	for name, p := range t.InputSchema.Properties {
		props[name] = property{Type: p.Type, Description: p.Description, Enum: p.Enum}
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

func (c *Client) toWingmanContentBlocks(blocks []contentBlock) []core.ContentBlock {
	result := make([]core.ContentBlock, len(blocks))
	for i, b := range blocks {
		result[i] = core.ContentBlock{
			Type:      core.ContentType(b.Type),
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

func (c *Client) toInferenceResponse(resp response) *core.InferenceResponse {
	return &core.InferenceResponse{
		ID:         resp.ID,
		Content:    c.toWingmanContentBlocks(resp.Content),
		StopReason: resp.StopReason,
		Usage: core.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}
}

// ============================================================
//  Provider interface implementation
// ============================================================

func (c *Client) buildRequest(req core.InferenceRequest) request {
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

	r := request{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		System:      req.Instructions,
		Messages:    messages,
		Tools:       tools,
	}

	if req.OutputSchema != nil {
		r.OutputConfig = &outputConfig{
			Format: &outputFormat{Type: "json_schema", Schema: req.OutputSchema},
		}
	}

	return r
}

// RunInference performs a blocking inference call.
func (c *Client) RunInference(ctx context.Context, req core.InferenceRequest) (*core.InferenceResponse, error) {
	anthropicReq := c.buildRequest(req)

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse response: %w", err)
	}

	return c.toInferenceResponse(apiResp), nil
}

// StreamInference begins a streaming inference call.
func (c *Client) StreamInference(ctx context.Context, req core.InferenceRequest) (core.Stream, error) {
	anthropicReq := c.buildRequest(req)
	anthropicReq.Stream = true

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: API error %d: %s", resp.StatusCode, string(body))
	}

	return newStream(resp), nil
}

// ============================================================
//  Streaming
// ============================================================

type Stream struct {
	resp         *http.Response
	scanner      *bufio.Scanner
	currentEvent core.StreamEvent
	err          error
	closed       bool

	accumulatedResponse *core.InferenceResponse
	contentBlocks       []core.ContentBlock
	currentBlockIndex   int
	currentBlockText    strings.Builder
	currentBlockJSON    strings.Builder
	currentToolUse      *core.ContentBlock
}

func newStream(resp *http.Response) *Stream {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return &Stream{
		resp:    resp,
		scanner: scanner,
		accumulatedResponse: &core.InferenceResponse{
			Content: []core.ContentBlock{},
		},
		contentBlocks: []core.ContentBlock{},
	}
}

func (s *Stream) Next() bool {
	if s.err != nil || s.closed {
		return false
	}

	var eventType string

	for s.scanner.Scan() {
		line := s.scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			event, done := s.parseEvent(eventType, data)
			if event != nil {
				s.currentEvent = *event
				return true
			}
			if done {
				return false
			}
		}
	}

	if err := s.scanner.Err(); err != nil {
		s.err = err
	}

	return false
}

func (s *Stream) parseEvent(eventType, data string) (*core.StreamEvent, bool) {
	switch eventType {
	case "message_start":
		var event struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("anthropic: failed to parse message_start: %w", err)
			return nil, true
		}
		s.accumulatedResponse.ID = event.Message.ID
		s.accumulatedResponse.Usage.InputTokens = event.Message.Usage.InputTokens
		return &core.StreamEvent{Type: core.EventMessageStart}, false

	case "content_block_start":
		var event struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type  string         `json:"type"`
				ID    string         `json:"id,omitempty"`
				Name  string         `json:"name,omitempty"`
				Text  string         `json:"text,omitempty"`
				Input map[string]any `json:"input,omitempty"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("anthropic: failed to parse content_block_start: %w", err)
			return nil, true
		}

		s.currentBlockIndex = event.Index
		s.currentBlockText.Reset()
		s.currentBlockJSON.Reset()

		if event.ContentBlock.Type == "tool_use" {
			s.currentToolUse = &core.ContentBlock{
				Type: core.ContentTypeToolUse,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		} else {
			s.currentToolUse = nil
		}

		return &core.StreamEvent{
			Type:  core.EventContentBlockStart,
			Index: event.Index,
			ContentBlock: &core.StreamContentBlock{
				Type: event.ContentBlock.Type,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
				Text: event.ContentBlock.Text,
			},
		}, false

	case "content_block_delta":
		var event struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text,omitempty"`
				PartialJSON string `json:"partial_json,omitempty"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("anthropic: failed to parse content_block_delta: %w", err)
			return nil, true
		}

		if event.Delta.Type == "text_delta" {
			s.currentBlockText.WriteString(event.Delta.Text)
			return &core.StreamEvent{
				Type:  core.EventTextDelta,
				Text:  event.Delta.Text,
				Index: event.Index,
			}, false
		} else if event.Delta.Type == "input_json_delta" {
			s.currentBlockJSON.WriteString(event.Delta.PartialJSON)
			return &core.StreamEvent{
				Type:      core.EventInputJSONDelta,
				InputJSON: event.Delta.PartialJSON,
				Index:     event.Index,
			}, false
		}
		return nil, false

	case "content_block_stop":
		var event struct {
			Index int `json:"index"`
		}
		json.Unmarshal([]byte(data), &event) //nolint:errcheck

		if s.currentToolUse != nil {
			var input map[string]any
			if s.currentBlockJSON.Len() > 0 {
				json.Unmarshal([]byte(s.currentBlockJSON.String()), &input) //nolint:errcheck
			}
			s.currentToolUse.Input = input
			s.contentBlocks = append(s.contentBlocks, *s.currentToolUse)
		} else if s.currentBlockText.Len() > 0 {
			s.contentBlocks = append(s.contentBlocks, core.ContentBlock{
				Type: core.ContentTypeText,
				Text: s.currentBlockText.String(),
			})
		}

		return &core.StreamEvent{
			Type:  core.EventContentBlockStop,
			Index: event.Index,
		}, false

	case "message_delta":
		var event struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("anthropic: failed to parse message_delta: %w", err)
			return nil, true
		}

		s.accumulatedResponse.StopReason = event.Delta.StopReason
		s.accumulatedResponse.Usage.OutputTokens = event.Usage.OutputTokens

		return &core.StreamEvent{
			Type:       core.EventMessageDelta,
			StopReason: event.Delta.StopReason,
			Usage: &core.Usage{
				InputTokens:  s.accumulatedResponse.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			},
		}, false

	case "message_stop":
		s.accumulatedResponse.Content = s.contentBlocks
		return &core.StreamEvent{Type: core.EventMessageStop}, true

	case "ping":
		return &core.StreamEvent{Type: core.EventPing}, false

	case "error":
		var event struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("anthropic: stream error: %s", data)
		} else {
			s.err = fmt.Errorf("anthropic: %s: %s", event.Error.Type, event.Error.Message)
		}
		return &core.StreamEvent{Type: core.EventError, Error: s.err}, true

	default:
		return nil, false
	}
}

func (s *Stream) Event() core.StreamEvent {
	return s.currentEvent
}

func (s *Stream) Err() error {
	return s.err
}

func (s *Stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func (s *Stream) Response() *core.InferenceResponse {
	return s.accumulatedResponse
}

var _ io.Closer = (*Stream)(nil)
