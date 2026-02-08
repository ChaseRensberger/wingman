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

// ============================================
//                    META
// ============================================

var Meta = provider.ProviderMeta{
	Name:        "anthropic",
	DisplayName: "Anthropic",
	AuthTypes:   []provider.AuthType{provider.AuthTypeAPIKey},
}

func init() {
	provider.Register(Meta)
}

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

const (
	defaultModel       = "claude-sonnet-4-5"
	defaultMaxTokens   = 8192
	defaultTemperature = 1.0
	apiURL             = "https://api.anthropic.com/v1/messages"
	apiVersion         = "2023-06-01"
	httpTimeout        = 5 * time.Minute
)

func New(cfg ...Config) *Client {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}

	apiKey := c.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := c.Model
	if model == "" {
		model = defaultModel
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	temperature := defaultTemperature
	if c.Temperature != nil {
		temperature = *c.Temperature
	}

	return &Client{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}
}

// ============================================
//                    TYPES
// ============================================

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
	Temperature  float64            `json:"temperature,omitempty"`
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

// ============================================
//
//	Type Conversions
//
// ============================================
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

// ============================================
//       Provider Interface Implementation
// ============================================

func (c *Client) RunInference(ctx context.Context, req models.WingmanInferenceRequest) (*models.WingmanInferenceResponse, error) {
	anthropicReq := c.buildRequest(req)

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

func (c *Client) buildRequest(req models.WingmanInferenceRequest) request {
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

	r := request{
		Model:       c.model,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      req.Instructions,
		Messages:    messages,
		Tools:       tools,
	}

	if req.OutputSchema != nil {
		r.OutputConfig = &outputConfig{
			Format: &outputFormat{
				Type:   "json_schema",
				Schema: req.OutputSchema,
			},
		}
	}

	return r
}

func (c *Client) StreamInference(ctx context.Context, req models.WingmanInferenceRequest) (provider.Stream, error) {
	anthropicReq := c.buildRequest(req)
	anthropicReq.Stream = true

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

	return newStream(resp), nil
}

// ============================================
//                   STREAMING
// ============================================

type Stream struct {
	resp         *http.Response
	scanner      *bufio.Scanner
	currentEvent models.StreamEvent
	err          error
	closed       bool

	accumulatedResponse *models.WingmanInferenceResponse
	contentBlocks       []models.WingmanContentBlock
	currentBlockIndex   int
	currentBlockText    strings.Builder
	currentBlockJSON    strings.Builder
	currentToolUse      *models.WingmanContentBlock
}

func newStream(resp *http.Response) *Stream {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return &Stream{
		resp:    resp,
		scanner: scanner,
		accumulatedResponse: &models.WingmanInferenceResponse{
			Content: []models.WingmanContentBlock{},
		},
		contentBlocks: []models.WingmanContentBlock{},
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

func (s *Stream) parseEvent(eventType, data string) (*models.StreamEvent, bool) {
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
			s.err = fmt.Errorf("failed to parse message_start: %w", err)
			return nil, true
		}
		s.accumulatedResponse.ID = event.Message.ID
		s.accumulatedResponse.Usage.InputTokens = event.Message.Usage.InputTokens
		return &models.StreamEvent{Type: models.EventMessageStart}, false

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
			s.err = fmt.Errorf("failed to parse content_block_start: %w", err)
			return nil, true
		}

		s.currentBlockIndex = event.Index
		s.currentBlockText.Reset()
		s.currentBlockJSON.Reset()

		if event.ContentBlock.Type == "tool_use" {
			s.currentToolUse = &models.WingmanContentBlock{
				Type: models.ContentTypeToolUse,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		} else {
			s.currentToolUse = nil
		}

		return &models.StreamEvent{
			Type:  models.EventContentBlockStart,
			Index: event.Index,
			ContentBlock: &models.StreamContentBlock{
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
			s.err = fmt.Errorf("failed to parse content_block_delta: %w", err)
			return nil, true
		}

		if event.Delta.Type == "text_delta" {
			s.currentBlockText.WriteString(event.Delta.Text)
			return &models.StreamEvent{
				Type:  models.EventTextDelta,
				Text:  event.Delta.Text,
				Index: event.Index,
			}, false
		} else if event.Delta.Type == "input_json_delta" {
			s.currentBlockJSON.WriteString(event.Delta.PartialJSON)
			return &models.StreamEvent{
				Type:      models.EventInputJSONDelta,
				InputJSON: event.Delta.PartialJSON,
				Index:     event.Index,
			}, false
		}
		return nil, false

	case "content_block_stop":
		var event struct {
			Index int `json:"index"`
		}
		json.Unmarshal([]byte(data), &event)

		if s.currentToolUse != nil {
			var input map[string]any
			if s.currentBlockJSON.Len() > 0 {
				json.Unmarshal([]byte(s.currentBlockJSON.String()), &input)
			}
			s.currentToolUse.Input = input
			s.contentBlocks = append(s.contentBlocks, *s.currentToolUse)
		} else if s.currentBlockText.Len() > 0 {
			s.contentBlocks = append(s.contentBlocks, models.WingmanContentBlock{
				Type: models.ContentTypeText,
				Text: s.currentBlockText.String(),
			})
		}

		return &models.StreamEvent{
			Type:  models.EventContentBlockStop,
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
			s.err = fmt.Errorf("failed to parse message_delta: %w", err)
			return nil, true
		}

		s.accumulatedResponse.StopReason = event.Delta.StopReason
		s.accumulatedResponse.Usage.OutputTokens = event.Usage.OutputTokens

		return &models.StreamEvent{
			Type:       models.EventMessageDelta,
			StopReason: event.Delta.StopReason,
			Usage: &models.WingmanUsage{
				InputTokens:  s.accumulatedResponse.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			},
		}, false

	case "message_stop":
		s.accumulatedResponse.Content = s.contentBlocks
		return &models.StreamEvent{Type: models.EventMessageStop}, true

	case "ping":
		return &models.StreamEvent{Type: models.EventPing}, false

	case "error":
		var event struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("stream error: %s", data)
		} else {
			s.err = fmt.Errorf("%s: %s", event.Error.Type, event.Error.Message)
		}
		return &models.StreamEvent{Type: models.EventError, Error: s.err}, true

	default:
		return nil, false
	}
}

func (s *Stream) Event() models.StreamEvent {
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

func (s *Stream) Response() *models.WingmanInferenceResponse {
	return s.accumulatedResponse
}

var _ io.Closer = (*Stream)(nil)
