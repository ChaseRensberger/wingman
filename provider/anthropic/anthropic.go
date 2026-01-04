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
	apiURL             = "https://api.anthropic.com/v1/messages"
	apiVersion         = "2023-06-01"
	httpTimeout        = 2 * time.Minute
)

type Config struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature *float64
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usage struct {
	InputTokens         int `json:"input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	OutputTokens        int `json:"output_tokens"`
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

type request struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
	Messages    []message `json:"messages"`
}

type Client struct {
	apiKey     string
	httpClient *http.Client
	defaults   request
}

func New(config Config) provider.ProviderFactory {
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

		return &Client{
			apiKey:     apiKey,
			httpClient: &http.Client{Timeout: httpTimeout},
			defaults: request{
				Model:       model,
				MaxTokens:   maxTokens,
				Temperature: temperature,
			},
		}, nil
	}
}

func (c *Client) RunInference(ctx context.Context, wingmanMessages []models.WingmanMessage, config models.WingmanConfig) (*models.WingmanMessageResponse, error) {
	messages := make([]message, len(wingmanMessages))
	for i, msg := range wingmanMessages {
		messages[i] = message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := c.defaults
	req.Messages = messages
	req.System = config.Instructions

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
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

	contentBlocks := make([]models.WingmanContentBlock, len(apiResp.Content))
	for i, block := range apiResp.Content {
		contentBlocks[i] = models.WingmanContentBlock{
			Type: block.Type,
			Text: block.Text,
		}
	}

	return &models.WingmanMessageResponse{
		ID:         apiResp.ID,
		Content:    contentBlocks,
		StopReason: apiResp.StopReason,
		Usage: models.WingmanUsage{
			InputTokens:  apiResp.Usage.InputTokens,
			OutputTokens: apiResp.Usage.OutputTokens,
		},
	}, nil
}
