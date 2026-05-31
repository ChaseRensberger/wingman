package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	webSearchDefaultProvider = "exa"
	webSearchTimeout         = 25 * time.Second
	webSearchExaURL          = "https://mcp.exa.ai/mcp"
	webSearchParallelURL     = "https://search.parallel.ai/mcp"
)

type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{}
}

func (t *WebSearchTool) Name() string {
	return "websearch"
}

func (t *WebSearchTool) Description() string {
	return fmt.Sprintf(`Search the web using Wingman's web search provider. Provides up-to-date information for current events and recent data. Use this tool for information beyond the model's knowledge cutoff.

Usage notes:
- Supports live crawling modes when available: "fallback" uses live crawling as backup if cached content is unavailable, "preferred" prioritizes live crawling.
- Supports search types when available: "auto" for balanced search, "fast" for quick results, "deep" for comprehensive search.
- The current year is %d. Use this year when searching for recent information or current events.`, time.Now().Year())
}

func (t *WebSearchTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"query": {
					Type:        "string",
					Description: "Web search query",
				},
				"numResults": {
					Type:        "number",
					Description: "Number of search results to return (default: 8)",
				},
				"livecrawl": {
					Type:        "string",
					Description: "Live crawl mode: fallback or preferred (default: fallback)",
					Enum:        []string{"fallback", "preferred"},
				},
				"type": {
					Type:        "string",
					Description: "Search type: auto, fast, or deep (default: auto)",
					Enum:        []string{"auto", "fast", "deep"},
				},
				"contextMaxCharacters": {
					Type:        "number",
					Description: "Maximum characters for context string optimized for LLMs (default: provider default)",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]any, workDir string) (Result, error) {
	query, ok := params["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return Result{}, fmt.Errorf("query is required")
	}
	query = strings.TrimSpace(query)

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("WINGMAN_WEBSEARCH_PROVIDER")))
	if provider == "" {
		provider = webSearchDefaultProvider
	}
	if provider != "exa" && provider != "parallel" {
		return Result{}, fmt.Errorf("unsupported websearch provider %q (expected exa or parallel)", provider)
	}

	body, endpoint, headers, err := webSearchRequest(provider, query, params)
	if err != nil {
		return Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("failed to create websearch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "wingman")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("websearch request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseSize+1)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read websearch response: %w", err)
	}
	if len(respBody) > maxResponseSize {
		return Result{}, fmt.Errorf("websearch response too large (exceeds 5MB limit)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("websearch request failed with status code %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	text, err := parseWebSearchResponse(string(respBody))
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(text) == "" {
		text = "No search results found. Please try a different query."
	}

	return Result{Text: text, Metadata: map[string]any{"provider": provider, "query": query}}, nil
}

func webSearchRequest(provider, query string, params map[string]any) ([]byte, string, map[string]string, error) {
	args := map[string]any{}
	toolName := ""
	endpoint := ""
	headers := map[string]string{}

	if provider == "parallel" {
		toolName = "web_search"
		endpoint = webSearchParallelURL
		args["objective"] = query
		args["search_queries"] = []string{query}
		if key := strings.TrimSpace(os.Getenv("PARALLEL_API_KEY")); key != "" {
			headers["Authorization"] = "Bearer " + key
		}
	} else {
		toolName = "web_search_exa"
		endpoint = webSearchExaEndpoint()
		args["query"] = query
		args["type"] = stringParam(params["type"], "auto")
		args["numResults"] = intParam(params["numResults"], 8)
		args["livecrawl"] = stringParam(params["livecrawl"], "fallback")
		if n := intParam(params["contextMaxCharacters"], 0); n > 0 {
			args["contextMaxCharacters"] = n
		}
	}

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to encode websearch request: %w", err)
	}
	return body, endpoint, headers, nil
}

func webSearchExaEndpoint() string {
	key := strings.TrimSpace(os.Getenv("EXA_API_KEY"))
	if key == "" {
		return webSearchExaURL
	}
	u, err := url.Parse(webSearchExaURL)
	if err != nil {
		return webSearchExaURL
	}
	q := u.Query()
	q.Set("exaApiKey", key)
	u.RawQuery = q.Encode()
	return u.String()
}

func parseWebSearchResponse(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "{") {
		return parseWebSearchPayload(trimmed)
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		text, err := parseWebSearchPayload(strings.TrimPrefix(line, "data: "))
		if err != nil {
			return "", err
		}
		if text != "" {
			return text, nil
		}
	}
	return "", nil
}

func parseWebSearchPayload(payload string) (string, error) {
	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		return "", fmt.Errorf("failed to parse websearch response: %w", err)
	}
	if response.Error != nil {
		return "", fmt.Errorf("websearch provider error: %s", response.Error.Message)
	}
	for _, item := range response.Result.Content {
		if item.Text != "" {
			return item.Text, nil
		}
	}
	return "", nil
}

func stringParam(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}
