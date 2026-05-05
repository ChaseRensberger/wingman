// Package opencodezen implements models.Model for the OpenCode Zen API.
//
// OpenCode Zen is a multi-model proxy at https://opencode.ai/zen/v1 that
// accepts OpenAI Chat Completions wire format for all models (including
// Claude and Gemini). Auth is a bearer OPENCODE_API_KEY.
//
// Catalog provider: "opencode" (see snapshot.json).
//
// All routing is handled by the upstream proxy; the client does not need to
// know which underlying model family it is talking to.
package opencodezen

import (
	"fmt"
	"os"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/providers/openaicompat"
)

const (
	baseURL       = "https://opencode.ai/zen/v1"
	providerID    = "opencodezen"
	catalogID     = "opencode"
	defaultModel  = "claude-sonnet-4-5"
)

// Meta is the registry entry for the OpenCode Zen provider.
var Meta = provider.ProviderMeta{
	ID:        providerID,
	Name:      "OpenCode Zen",
	AuthTypes: []provider.AuthType{provider.AuthTypeAPIKey},
	Factory: func(opts map[string]any) (models.Model, error) {
		return New(Config{Options: opts})
	},
}

func init() { provider.Register(Meta) }

// Config controls Client construction.
type Config struct {
	APIKey    string
	Model     string
	MaxTokens int
	Options   map[string]any
}

// New constructs a Client. API key resolution: Config.APIKey → Options["api_key"]
// → OPENCODE_API_KEY env var.
func New(cfg ...Config) (*openaicompat.Client, error) {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}

	apiKey := c.APIKey
	if apiKey == "" {
		if k, ok := c.Options["api_key"].(string); ok && k != "" {
			apiKey = k
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("opencodezen: no API key (set Config.APIKey, Options[\"api_key\"], or OPENCODE_API_KEY)")
	}

	model := c.Model
	if model == "" {
		if m, ok := c.Options["model"].(string); ok && m != "" {
			model = m
		}
	}
	if model == "" {
		model = defaultModel
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		if v, ok := c.Options["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				maxTokens = n
			case float64:
				maxTokens = int(n)
			}
		}
	}

	return openaicompat.New(openaicompat.Config{
		ProviderID:      providerID,
		CatalogProvider: catalogID,
		APIKey:          apiKey,
		Model:           model,
		BaseURL:         baseURL,
		MaxTokens:       maxTokens,
		Options:         c.Options,
	})
}
