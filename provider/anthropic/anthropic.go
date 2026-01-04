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

	"wingman/models"
	"wingman/provider"
)

const (
	defaultModel       = "claude-haiku-4-5-20251001"
	defaultMaxTokens   = 4096
	defaultTemperature = 1.0
	httpTimeout        = 2 * time.Minute
)

type AnthropicConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature *float64
}

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

func New(config AnthropicConfig) provider.ProviderFactory {
	return func(wingmanConfig models.WingmanConfig) (provider.InferenceProvider, error) {
		apiKey := config.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
		}

		model := config.Model
		if model == "" {
			model = defaultModel
		}

		maxTokens := config.MaxTokens
		if maxTokens <= 0 {
			maxTokens = wingmanConfig.MaxTokens
		}
		if maxTokens <= 0 {
			maxTokens = defaultMaxTokens
		}

		temperature := defaultTemperature
		if config.Temperature != nil {
			temperature = *config.Temperature
		} else if wingmanConfig.Temperature != nil {
			temperature = *wingmanConfig.Temperature
		}

		return &AnthropicClient{
			apiKey:      apiKey,
			model:       model,
			maxTokens:   maxTokens,
			temperature: temperature,
			httpClient: &http.Client{
				Timeout: httpTimeout,
			},
		}, nil
	}
}

func (ac *AnthropicClient) RunInference(ctx context.Context, wingmanMessages []models.WingmanMessage, config models.WingmanConfig) (*models.WingmanMessageResponse, error) {
	anthropicMessages := make([]AnthropicMessage, len(wingmanMessages))
	for i, msg := range wingmanMessages {
		anthropicMessages[i] = AnthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := AnthropicMessageRequest{
		Model:       ac.model,
		MaxTokens:   ac.maxTokens,
		Temperature: ac.temperature,
		Messages:    anthropicMessages,
		System:      config.Instructions,
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

	var anthropicResp AnthropicMessageResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	wingmanContentBlocks := make([]models.WingmanContentBlock, len(anthropicResp.Content))
	for i, block := range anthropicResp.Content {
		wingmanContentBlocks[i] = models.WingmanContentBlock{
			Type: block.Type,
			Text: block.Text,
		}
	}

	wingmanResp := &models.WingmanMessageResponse{
		ID:         anthropicResp.ID,
		Content:    wingmanContentBlocks,
		StopReason: anthropicResp.StopReason,
		Usage: models.WingmanUsage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}

	return wingmanResp, nil
}
