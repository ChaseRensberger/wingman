package openaicompat

import provider "github.com/chaserensberger/wingman/models/providers"

type profile struct {
	id      string
	name    string
	baseURL string
}

func init() {
	for _, p := range []profile{
		{id: "openai-compatible", name: "OpenAI Compatible"},
		{id: "xai", name: "xAI", baseURL: "https://api.x.ai/v1"},
	} {
		meta := provider.ProviderMeta{ID: p.id, Name: p.name, BaseURL: p.baseURL}
		meta.AuthTypes = []provider.AuthType{{Type: "api_key"}}
		provider.Register(meta)
	}
}
