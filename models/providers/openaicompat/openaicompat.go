// Package openaicompat implements models.Model for providers that speak the
// OpenAI Chat Completions wire format (/v1/chat/completions).
package openaicompat

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/protocols/openaichat"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/route"
)

// Config controls construction of a Client.
type Config struct {
	// ProviderID is the registry key (e.g. "openai", "opencodezen", "deepseek").
	ProviderID string
	// CatalogProvider is the catalog lookup key. Falls back to ProviderID if empty.
	CatalogProvider string
	APIKey          string
	Model           string
	BaseURL         string
	MaxTokens       int
	MaxRetries      int
	Options         map[string]any
}

const (
	defaultMaxTokens  = 4096
	defaultMaxRetries = 3
	httpTimeout       = 5 * time.Minute
	maxRetryDelay     = 60 * time.Second
)

// Client is a configured OpenAI-compatible Model. It is a thin provider facade
// over the reusable OpenAI Chat protocol plus route transport/auth concerns.
type Client struct {
	providerID      string
	catalogProvider string
	apiKey          string
	model           string
	baseURL         string
	maxTokens       int
	httpClient      *http.Client
	maxRetries      int
	route           route.Route
}

// New constructs a Client. API key resolution order: Config.APIKey,
// Options["api_key"], then <PROVIDER_ID_UPPER>_API_KEY env var.
func New(cfg Config) (*Client, error) {
	providerID := cfg.ProviderID
	if providerID == "" {
		providerID = "openaicompat"
	}
	catalogProvider := cfg.CatalogProvider
	if catalogProvider == "" {
		catalogProvider = providerID
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		if k, ok := cfg.Options["api_key"].(string); ok && k != "" {
			apiKey = k
		}
	}
	if apiKey == "" {
		envKey := strings.ToUpper(strings.ReplaceAll(providerID, "-", "_")) + "_API_KEY"
		apiKey = os.Getenv(envKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%s: no API key (set Options[\"api_key\"] or %s_API_KEY)",
			providerID, strings.ToUpper(strings.ReplaceAll(providerID, "-", "_")))
	}

	model := cfg.Model
	if model == "" {
		if m, ok := cfg.Options["model"].(string); ok && m != "" {
			model = m
		}
	}
	if model == "" {
		return nil, fmt.Errorf("%s: no model specified", providerID)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if u, ok := cfg.Options["base_url"].(string); ok && u != "" {
			baseURL = u
		}
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%s: no base_url specified", providerID)
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		if v, ok := cfg.Options["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				maxTokens = n
			case float64:
				maxTokens = int(n)
			}
		}
	}
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	c := &Client{
		providerID:      providerID,
		catalogProvider: catalogProvider,
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		maxTokens:       maxTokens,
		httpClient:      &http.Client{Timeout: httpTimeout},
		maxRetries:      maxRetries,
	}
	c.route = route.Route{
		ID:        "openai-compatible-chat",
		Provider:  providerID,
		Protocol:  openaichat.Protocol{},
		Endpoint:  route.Path("/chat/completions"),
		Auth:      route.Bearer(apiKey),
		Transport: route.HTTPTransport{Client: c.httpClient, MaxRetries: c.maxRetries, MaxRetryDelay: maxRetryDelay},
	}
	return c, nil
}

// Info returns catalog ModelInfo. API is always APIOpenAICompletions.
func (c *Client) Info() models.ModelInfo {
	if info, ok := catalog.Get(c.catalogProvider, c.model); ok {
		info.API = models.APIOpenAICompletions
		info.BaseURL = c.baseURL
		return info
	}
	return models.ModelInfo{Provider: c.providerID, ID: c.model, API: models.APIOpenAICompletions, BaseURL: c.baseURL}
}

func (c *Client) CountTokens(_ context.Context, msgs []models.Message) (int, error) {
	return openaichat.CountTokens(msgs), nil
}

func (c *Client) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return c.route.Stream(ctx, c.modelRef(), req)
}

// Prepare lowers a provider-neutral request into the HTTP request that Stream
// would send, without performing any network I/O.
func (c *Client) Prepare(ctx context.Context, req models.Request) (*route.PreparedRequest, error) {
	return c.route.Prepare(ctx, c.modelRef(), req)
}

func (c *Client) modelRef() route.ModelRef {
	return route.ModelRef{
		Provider:        c.providerID,
		ModelID:         c.model,
		BaseURL:         c.baseURL,
		MaxOutputTokens: c.maxTokens,
		Info:            c.Info(),
	}
}

// RegisterProvider registers an OpenAI-compatible provider with the default
// registry. The factory reads model, base_url, api_key from opts.
// catalogProvider overrides the catalog lookup key (defaults to providerID).
func RegisterProvider(providerID, catalogProvider, defaultBaseURL, envKeyVar string, authTypes []provider.AuthType) {
	provider.Register(provider.ProviderMeta{
		ID:        providerID,
		Name:      providerID,
		AuthTypes: authTypes,
		Factory: func(opts map[string]any) (models.Model, error) {
			apiKey := ""
			if k, ok := opts["api_key"].(string); ok {
				apiKey = k
			}
			if apiKey == "" {
				apiKey = os.Getenv(envKeyVar)
			}
			model := ""
			if m, ok := opts["model"].(string); ok {
				model = m
			}
			baseURL := defaultBaseURL
			if u, ok := opts["base_url"].(string); ok && u != "" {
				baseURL = u
			}
			return New(Config{
				ProviderID:      providerID,
				CatalogProvider: catalogProvider,
				APIKey:          apiKey,
				Model:           model,
				BaseURL:         baseURL,
				Options:         opts,
			})
		},
	})
}

var _ models.Model = (*Client)(nil)
