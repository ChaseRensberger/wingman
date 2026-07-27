// Package provider is the global model provider registry and default client.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers/internal/httpmodel"
)

// AuthType describes a supported authentication scheme.
type AuthType struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ProviderMeta describes a registered provider.
type ProviderMeta struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	AuthTypes []AuthType `json:"auth_types,omitempty"`
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProviderMeta)
)

// Register adds a provider to the global registry. Overwrites existing entries.
func Register(meta ProviderMeta) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[meta.ID] = meta
}

// List returns all registered providers in an unspecified order.
func List() []ProviderMeta {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]ProviderMeta, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	return out
}

// Get returns the metadata for a provider by ID.
func Get(id string) (ProviderMeta, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	m, ok := registry[id]
	if !ok {
		return ProviderMeta{}, fmt.Errorf("unknown provider: %s", id)
	}
	return m, nil
}

// IsValid reports whether a provider ID is registered.
func IsValid(id string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[id]
	return ok
}

// Client resolves catalog model refs and explicit custom model routes.
type Client struct {
	Auth        map[string]string
	Credentials map[string]Credential
	Refresh     func(context.Context, string, Credential) (Credential, error)
	Providers   map[string]ProviderConfig
}

// Credential is one provider credential resolved by a caller-owned auth store.
type Credential struct {
	Type      string
	Key       string
	Access    string
	Refresh   string
	ExpiresAt int64
	AccountID string
}

// ProviderConfig overlays catalog provider behavior for one process.
type ProviderConfig struct {
	Name      string                      `json:"name,omitempty"`
	AuthTypes []AuthType                  `json:"auth_types,omitempty"`
	Options   ProviderOptions             `json:"options,omitempty"`
	Models    map[string]models.ModelInfo `json:"models,omitempty"`
}

// ProviderOptions are runtime options for a provider route.
type ProviderOptions struct {
	BaseURL    string            `json:"baseURL,omitempty"`
	Auth       *bool             `json:"auth,omitempty"`
	AuthHeader string            `json:"authHeader,omitempty"`
	AuthScheme string            `json:"authScheme,omitempty"`
	Query      map[string]string `json:"query,omitempty"`
}

// NewClient constructs a route-backed provider client.
func NewClient(auth map[string]string) *Client {
	return &Client{Auth: auth}
}

// NewClientWithConfig constructs a route-backed provider client with
// process-local provider overlays.
func NewClientWithConfig(auth map[string]string, providers map[string]ProviderConfig) *Client {
	return &Client{Auth: auth, Providers: providers}
}

// NewClientWithCredentials constructs a route-backed client with richer stored credentials.
func NewClientWithCredentials(credentials map[string]Credential, providers map[string]ProviderConfig, refresh func(context.Context, string, Credential) (Credential, error)) *Client {
	return &Client{Credentials: credentials, Providers: providers, Refresh: refresh}
}

// RegisterConfig adds config-defined providers and model metadata for this process.
// Existing provider IDs keep their registered metadata unless config supplies fields.
func RegisterConfig(providers map[string]ProviderConfig) {
	for id, cfg := range providers {
		if id == "" {
			continue
		}
		meta, err := Get(id)
		if err != nil {
			meta = ProviderMeta{ID: id, Name: id, AuthTypes: []AuthType{{Type: "api_key"}}}
		}
		if cfg.Name != "" {
			meta.Name = cfg.Name
		}
		if len(cfg.AuthTypes) > 0 {
			meta.AuthTypes = cfg.AuthTypes
		}
		Register(meta)
		if len(cfg.Models) > 0 {
			catalog.RegisterProviderOverlay(id, cfg.Options.BaseURL, cfg.Models)
		}
	}
}

// Prepare lowers a request into provider-native JSON without sending it.
func (c *Client) Prepare(ctx context.Context, req models.Request) (*models.PreparedRequest, error) {
	m, err := c.model(req.Model)
	if err != nil {
		return nil, err
	}
	return m.Prepare(ctx, req)
}

// Stream sends the request to the selected provider route.
func (c *Client) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	m, err := c.model(req.Model)
	if err != nil {
		return nil, err
	}
	return m.Stream(ctx, req)
}

// Generate drains Stream and returns the final assistant message.
func (c *Client) Generate(ctx context.Context, req models.Request) (*models.Message, error) {
	return models.Generate(ctx, c, req)
}

func (c *Client) model(ref models.ModelRef) (*httpmodel.Model, error) {
	info, err := resolveModelInfo(ref)
	if err != nil {
		return nil, err
	}
	var cfg ProviderConfig
	if providerCfg, ok := c.Providers[info.Provider]; ok {
		cfg = providerCfg
		if cfg.Options.BaseURL != "" {
			info.BaseURL = cfg.Options.BaseURL
		}
	}
	protocol, err := protocolFor(info.API)
	if err != nil {
		return nil, err
	}
	apiKey := ""
	credential := c.Credentials[info.Provider]
	useAuth := true
	if cfg.Options.Auth != nil {
		useAuth = *cfg.Options.Auth
	}
	if useAuth {
		if credential.Type == "api_key" {
			apiKey = credential.Key
		} else if c.Auth != nil {
			apiKey = c.Auth[info.Provider]
		}
		if apiKey == "" {
			for _, env := range info.Env {
				if v := os.Getenv(env); v != "" {
					apiKey = v
					break
				}
			}
		}
	}
	if credential.Type == "oauth" && info.Provider == "openai" {
		info.BaseURL = "https://chatgpt.com/backend-api/codex"
	}
	query := cfg.Options.Query
	if protocol == httpmodel.GeminiGenerate {
		query = map[string]string{"alt": "sse"}
		for k, v := range cfg.Options.Query {
			query[k] = v
		}
	}
	return &httpmodel.Model{
		Info_:           info,
		Protocol:        protocol,
		BaseURL:         info.BaseURL,
		APIKey:          apiKey,
		ForceStoreFalse: credential.Type == "oauth" && info.Provider == "openai",
		Route: &httpmodel.Route{
			ID:       string(protocol),
			Protocol: protocol,
			Endpoint: httpmodel.Endpoint{BaseURL: info.BaseURL, Query: query, ModelID: info.ID},
			Auth:     c.routeAuth(protocol, info.Provider, apiKey, credential, cfg.Options),
			Headers:  routeHeaders(protocol, credential),
		},
	}, nil
}

func (c *Client) routeAuth(protocol httpmodel.Protocol, providerID, apiKey string, credential Credential, options ProviderOptions) httpmodel.Auth {
	if options.Auth != nil && !*options.Auth {
		return httpmodel.NoAuth
	}
	if credential.Type == "oauth" && providerID == "openai" {
		return httpmodel.AuthFunc(func(req *http.Request) error {
			current := credential
			if current.Access == "" || current.ExpiresAt <= time.Now().Unix() {
				if c.Refresh == nil {
					return fmt.Errorf("openai OAuth token is expired; reconnect the provider")
				}
				var err error
				current, err = c.Refresh(req.Context(), providerID, current)
				if err != nil {
					return err
				}
			}
			if current.Access == "" {
				return fmt.Errorf("openai OAuth access token is missing")
			}
			req.Header.Set("authorization", "Bearer "+current.Access)
			if current.AccountID != "" {
				req.Header.Set("ChatGPT-Account-Id", current.AccountID)
			}
			return nil
		})
	}
	if apiKey == "" {
		return httpmodel.NoAuth
	}
	header := options.AuthHeader
	if header == "" && protocol == httpmodel.AnthropicMessages {
		header = "x-api-key"
	}
	if header == "" && protocol == httpmodel.GeminiGenerate {
		header = "x-goog-api-key"
	}
	if header != "" {
		value := apiKey
		if options.AuthScheme != "" {
			value = options.AuthScheme + " " + apiKey
		}
		return httpmodel.HeaderAuth(header, value)
	}
	return httpmodel.BearerAuth(apiKey)
}

func routeHeaders(protocol httpmodel.Protocol, credential Credential) map[string]string {
	if credential.Type == "oauth" {
		return map[string]string{"originator": "codex_cli_rs"}
	}
	if protocol == httpmodel.AnthropicMessages {
		return map[string]string{
			"anthropic-version": "2023-06-01",
			"anthropic-beta":    "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14",
		}
	}
	return nil
}

func resolveModelInfo(ref models.ModelRef) (models.ModelInfo, error) {
	if ref.Provider == "" || ref.ID == "" {
		return models.ModelInfo{}, fmt.Errorf("model ref is required")
	}
	if info, ok := catalog.Get(ref.Provider, ref.ID); ok {
		return info, nil
	}
	if ref.API == "" || ref.BaseURL == "" {
		return models.ModelInfo{}, fmt.Errorf("unknown model: %s; provide api and base_url for custom models", ref.Ref())
	}
	return models.ModelInfo{
		Provider:      ref.Provider,
		ID:            ref.ID,
		API:           ref.API,
		BaseURL:       ref.BaseURL,
		Env:           ref.Env,
		ContextWindow: ref.ContextWindow,
		MaxOutput:     ref.MaxOutput,
		Capabilities:  ref.Capabilities,
	}, nil
}

func protocolFor(api models.API) (httpmodel.Protocol, error) {
	switch api {
	case models.APIOpenAIResponses:
		return httpmodel.OpenAIResponses, nil
	case models.APIOpenAICompletions:
		return httpmodel.OpenAIChat, nil
	case models.APIOpenAICompatible:
		return httpmodel.OpenAIChat, nil
	case models.APIAnthropicMessages:
		return httpmodel.AnthropicMessages, nil
	case models.APIGeminiGenerate:
		return httpmodel.GeminiGenerate, nil
	default:
		return "", fmt.Errorf("unsupported model API: %s", api)
	}
}
