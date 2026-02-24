package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/chaserensberger/wingman/core"
)

const perplexityBaseURL = "https://api.perplexity.ai"

type PerplexityTool struct {
	apiKey string
}

func NewPerplexityTool() *PerplexityTool {
	return &PerplexityTool{
		apiKey: os.Getenv("PERPLEXITY_API_KEY"),
	}
}

func (t *PerplexityTool) Name() string {
	return "perplexity_search"
}

func (t *PerplexityTool) Description() string {
	return "Search the web using Perplexity AI. Returns real-time search results with titles, URLs, snippets, and dates."
}

func (t *PerplexityTool) Definition() core.ToolDefinition {
	return core.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: core.ToolInputSchema{
			Type: "object",
			Properties: map[string]core.ToolProperty{
				"query": {
					Type:        "string",
					Description: "The search query to send to Perplexity",
				},
			},
			Required: []string{"query"},
		},
	}
}

type perplexitySearchRequest struct {
	Query            string `json:"query"`
	MaxResults       int    `json:"max_results"`
	MaxTokensPerPage int    `json:"max_tokens_per_page"`
}

type perplexitySearchResponse struct {
	Results []perplexitySearchResult `json:"results,omitempty"`
	Answer  string                   `json:"answer,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

type perplexitySearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	Date        string `json:"date"`
	LastUpdated string `json:"last_updated"`
}

func (t *PerplexityTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	if t.apiKey == "" {
		return "", fmt.Errorf("PERPLEXITY_API_KEY environment variable is not set")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query is required")
	}

	reqBody := perplexitySearchRequest{
		Query:            query,
		MaxResults:       5,
		MaxTokensPerPage: 1024,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", perplexityBaseURL+"/search", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("perplexity API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result perplexitySearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != "" {
		return "", fmt.Errorf("perplexity search error: %s", result.Error)
	}

	formatted, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return string(formatted), nil
}
