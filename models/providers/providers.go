// Package provider provides immutable model-provider generations and clients.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
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
	BaseURL   string     `json:"base_url,omitempty"`
	AuthTypes []AuthType `json:"auth_types,omitempty"`
}

var (
	registryMu        sync.RWMutex
	registry          = make(map[string]ProviderMeta)
	registryFrozen    bool
	builtinOnce       sync.Once
	builtinGeneration *Registry
)

// Register adds built-in provider metadata during package initialization. It
// panics after the first registry generation freezes the built-in snapshot.
func Register(meta ProviderMeta) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if registryFrozen {
		panic("provider: built-in registry is frozen")
	}
	registry[meta.ID] = meta
}

// List returns all registered built-in providers in deterministic ID order.
func List() []ProviderMeta {
	return builtinRegistry().List()
}

// Get returns the metadata for a provider by ID.
func Get(id string) (ProviderMeta, error) {
	return builtinRegistry().Get(id)
}

// IsValid reports whether a provider ID is registered.
func IsValid(id string) bool {
	return builtinRegistry().IsValid(id)
}

// Client resolves catalog model refs and explicit custom model routes.
type Client struct {
	Auth        map[string]string
	Credentials map[string]Credential
	Refresh     func(context.Context, string, Credential) (Credential, error)
	registry    *Registry
}

// Registry is an immutable provider and catalog generation.
type Registry struct {
	providers map[string]ProviderMeta
	catalog   *catalog.Catalog
	configs   map[string]ProviderConfig
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

// NewRegistry creates an immutable generation from built-in providers and config.
func NewRegistry(configs map[string]ProviderConfig) (*Registry, error) {
	registryMu.Lock()
	registryFrozen = true
	metas := make(map[string]ProviderMeta, len(registry))
	for id, meta := range registry {
		metas[id] = cloneMeta(meta)
	}
	registryMu.Unlock()

	overlays := map[string]catalog.ProviderOverlay{}
	for id, meta := range metas {
		if meta.BaseURL != "" {
			overlays[id] = catalog.ProviderOverlay{BaseURL: meta.BaseURL}
		}
	}
	ids := make([]string, 0, len(configs))
	for id := range configs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	snapshot := make(map[string]ProviderConfig, len(configs))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("provider ID is required")
		}
		cfg := cloneConfig(configs[id])
		meta, exists := metas[id]
		if !exists {
			meta = ProviderMeta{ID: id, Name: id, AuthTypes: []AuthType{{Type: "api_key"}}}
		}
		if cfg.Name != "" {
			meta.Name = cfg.Name
		}
		if len(cfg.AuthTypes) > 0 {
			meta.AuthTypes = append([]AuthType(nil), cfg.AuthTypes...)
		}
		baseURL := cfg.Options.BaseURL
		if baseURL == "" {
			baseURL, _ = catalog.GetProviderBaseURL(id)
		}
		if baseURL == "" {
			baseURL = meta.BaseURL
		}
		modelIDs := make([]string, 0, len(cfg.Models))
		for modelID := range cfg.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)
		for _, modelID := range modelIDs {
			info := cfg.Models[modelID]
			if modelID == "" {
				return nil, fmt.Errorf("provider %q: model ID is required", id)
			}
			if info.Provider == "" {
				info.Provider = id
			}
			if info.ID == "" {
				info.ID = modelID
			}
			if info.Provider != id || info.ID != modelID {
				return nil, fmt.Errorf("provider %q: model %q identity does not match its map key", id, modelID)
			}
			if _, err := protocolFor(info.API); err != nil {
				return nil, fmt.Errorf("provider %q model %q: %w", id, modelID, err)
			}
			if info.BaseURL == "" {
				info.BaseURL = baseURL
			}
			if info.BaseURL == "" {
				return nil, fmt.Errorf("provider %q model %q: base URL is required", id, modelID)
			}
			cfg.Models[modelID] = info
		}
		meta.BaseURL = baseURL
		metas[id] = meta
		overlays[id] = catalog.ProviderOverlay{BaseURL: baseURL, Models: cfg.Models}
		snapshot[id] = cfg
	}
	c, err := catalog.New(overlays)
	if err != nil {
		return nil, err
	}
	return &Registry{providers: metas, catalog: c, configs: snapshot}, nil
}

// Catalog returns this generation's immutable catalog snapshot.
func (r *Registry) Catalog() *catalog.Catalog { return r.catalog }

// List returns generation providers in deterministic ID order.
func (r *Registry) List() []ProviderMeta {
	out := make([]ProviderMeta, 0, len(r.providers))
	for _, meta := range r.providers {
		out = append(out, cloneMeta(meta))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns a generation provider by ID.
func (r *Registry) Get(id string) (ProviderMeta, error) {
	meta, ok := r.providers[id]
	if !ok {
		return ProviderMeta{}, fmt.Errorf("unknown provider: %s", id)
	}
	return cloneMeta(meta), nil
}

// IsValid reports whether a provider ID exists in this generation.
func (r *Registry) IsValid(id string) bool { _, ok := r.providers[id]; return ok }

// Config returns the immutable authored overlay for one provider.
func (r *Registry) Config(id string) (ProviderConfig, bool) {
	cfg, ok := r.configs[id]
	return cloneConfig(cfg), ok
}

// NewClient creates a credential-keyed client for this generation.
func (r *Registry) NewClient(auth map[string]string) *Client { return &Client{Auth: auth, registry: r} }

// NewClientWithCredentials creates a credential-backed client for this generation.
func (r *Registry) NewClientWithCredentials(credentials map[string]Credential, refresh func(context.Context, string, Credential) (Credential, error)) *Client {
	return &Client{Credentials: credentials, Refresh: refresh, registry: r}
}

func builtinRegistry() *Registry {
	builtinOnce.Do(func() {
		var err error
		builtinGeneration, err = NewRegistry(nil)
		if err != nil {
			panic(err)
		}
	})
	return builtinGeneration
}

// NewClient constructs a route-backed client using built-in providers.
func NewClient(auth map[string]string) *Client {
	return builtinRegistry().NewClient(auth)
}

// NewClientWithCredentials constructs a built-in client with richer stored credentials.
func NewClientWithCredentials(credentials map[string]Credential, refresh func(context.Context, string, Credential) (Credential, error)) *Client {
	return builtinRegistry().NewClientWithCredentials(credentials, refresh)
}

// Prepare lowers a request into provider-native JSON without sending it.
func (c *Client) Prepare(ctx context.Context, req models.Request) (*models.PreparedRequest, error) {
	m, err := c.model(req.Model)
	if err != nil {
		return nil, err
	}
	return m.Prepare(ctx, req)
}

// LoweredOptions reports safe provider-specific request options for a model call.
func (c *Client) LoweredOptions(ctx context.Context, req models.Request) models.LoweredOptions {
	m, err := c.model(req.Model)
	if err != nil {
		return models.LoweredOptions{}
	}
	return m.LoweredOptions(ctx, req)
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
	info, err := resolveModelInfo(c.registry.catalog, ref)
	if err != nil {
		return nil, err
	}
	var variant models.ModelVariant
	if ref.Variant != "" {
		var ok bool
		variant, ok = info.Variant(ref.Variant)
		if !ok {
			return nil, fmt.Errorf("variant unavailable for %s/%s: %s", ref.Provider, ref.ID, ref.Variant)
		}
	}
	var cfg ProviderConfig
	if providerCfg, ok := c.registry.configs[info.Provider]; ok {
		cfg = providerCfg
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
		Variant:         variant,
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

func resolveModelInfo(c *catalog.Catalog, ref models.ModelRef) (models.ModelInfo, error) {
	if ref.Provider == "" || ref.ID == "" {
		return models.ModelInfo{}, fmt.Errorf("model ref is required")
	}
	if info, ok := c.Get(ref.Provider, ref.ID); ok {
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

func cloneMeta(meta ProviderMeta) ProviderMeta {
	meta.AuthTypes = append([]AuthType(nil), meta.AuthTypes...)
	return meta
}

func cloneConfig(cfg ProviderConfig) ProviderConfig {
	cfg.AuthTypes = append([]AuthType(nil), cfg.AuthTypes...)
	cfg.Options.Query = cloneStrings(cfg.Options.Query)
	if cfg.Options.Auth != nil {
		auth := *cfg.Options.Auth
		cfg.Options.Auth = &auth
	}
	modelsByID := cfg.Models
	cfg.Models = make(map[string]models.ModelInfo, len(modelsByID))
	for id, info := range modelsByID {
		info.Env = append([]string(nil), info.Env...)
		info.Variants = cloneVariants(info.Variants)
		cfg.Models[id] = info
	}
	return cfg
}

func cloneVariants(in []models.ModelVariant) []models.ModelVariant {
	if in == nil {
		return nil
	}
	out := make([]models.ModelVariant, len(in))
	for i, variant := range in {
		out[i] = variant
		out[i].ProviderOptions = cloneJSONMap(variant.ProviderOptions)
		out[i].HTTP.Headers = cloneStrings(variant.HTTP.Headers)
		out[i].HTTP.Query = cloneStrings(variant.HTTP.Query)
		out[i].HTTP.Body = cloneJSONMap(variant.HTTP.Body)
	}
	return out
}

func cloneJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneJSONMap(value)
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = cloneJSONValue(item)
		}
		return out
	default:
		return value
	}
}

func cloneStrings(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
