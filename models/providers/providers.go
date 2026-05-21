// Package provider is the global model provider registry and default client.
package provider

import (
	"context"
	"fmt"
	"os"
	"sync"

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
	Auth      map[string]string
	Providers map[string]ProviderConfig
}

// ProviderConfig overlays catalog provider behavior for one process.
type ProviderConfig struct {
	Options ProviderOptions `json:"options,omitempty"`
}

// ProviderOptions are runtime options for a provider route.
type ProviderOptions struct {
	BaseURL string `json:"baseURL,omitempty"`
	Auth    *bool  `json:"auth,omitempty"`
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
	if cfg, ok := c.Providers[info.Provider]; ok {
		if cfg.Options.BaseURL != "" {
			info.BaseURL = cfg.Options.BaseURL
		}
	}
	protocol, err := protocolFor(info.API)
	if err != nil {
		return nil, err
	}
	apiKey := ""
	useAuth := true
	if cfg, ok := c.Providers[info.Provider]; ok && cfg.Options.Auth != nil {
		useAuth = *cfg.Options.Auth
	}
	if useAuth {
		if c.Auth != nil {
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
	return &httpmodel.Model{
		Info_:    info,
		Protocol: protocol,
		BaseURL:  info.BaseURL,
		APIKey:   apiKey,
	}, nil
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
	case models.APIAnthropicMessages:
		return httpmodel.AnthropicMessages, nil
	default:
		return "", fmt.Errorf("unsupported model API: %s", api)
	}
}
