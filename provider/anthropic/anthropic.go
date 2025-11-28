package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens         int `json:"input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	OutputTokens        int `json:"output_tokens"`
}

type AnthropicMessageResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Model        string                  `json:"model"`
	Content      []AnthropicContentBlock `json:"content"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicMessageRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	System      string             `json:"system,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
}

type AnthropicClient struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

func CreateAnthropicClient(config map[string]any) (*AnthropicClient, error) {
	apiKey := ""
	if key, ok := config["api_key"].(string); ok {
		apiKey = key
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	model := "claude-haiku-4-5-20251001"
	if m, ok := config["model"].(string); ok && m != "" {
		model = m
	}

	maxTokens := 4096
	if mt, ok := config["max_tokens"].(int); ok && mt > 0 {
		maxTokens = mt
	}

	temperature := 1.
	if temp, ok := config["temperature"].(float64); ok && temp > 0 {
		temperature = temp
	}

	return &AnthropicClient{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}, nil
}

func (ac *AnthropicClient) RunInference(ctx context.Context, input any) (any, error) {
	messages, ok := input.([]AnthropicMessage)
	if !ok {
		return nil, fmt.Errorf("input must be []AnthropicMessage")
	}

	req := AnthropicMessageRequest{
		Model:       ac.model,
		MaxTokens:   ac.maxTokens,
		Temperature: ac.temperature,
		Messages:    messages,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", ac.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := ac.httpClient.Do(httpReq)
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

	var msgResp AnthropicMessageResponse
	if err := json.Unmarshal(body, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &msgResp, nil
}
