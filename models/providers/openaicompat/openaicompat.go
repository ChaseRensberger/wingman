package openaicompat

import (
	"github.com/chaserensberger/wingman/models/catalog"
	provider "github.com/chaserensberger/wingman/models/providers"
)

type profile struct {
	id      string
	name    string
	baseURL string
}

func init() {
	for _, p := range []profile{
		{id: "openai-compatible", name: "OpenAI Compatible"},
		{id: "openrouter", name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1"},
		{id: "groq", name: "Groq", baseURL: "https://api.groq.com/openai/v1"},
		{id: "togetherai", name: "Together AI", baseURL: "https://api.together.xyz/v1"},
		{id: "deepseek", name: "DeepSeek", baseURL: "https://api.deepseek.com/v1"},
		{id: "xai", name: "xAI", baseURL: "https://api.x.ai/v1"},
	} {
		meta := provider.ProviderMeta{ID: p.id, Name: p.name}
		meta.AuthTypes = []provider.AuthType{{Type: "api_key"}}
		provider.Register(meta)
		if p.baseURL != "" {
			catalog.RegisterProviderOverlay(p.id, p.baseURL, nil)
		}
	}
}
