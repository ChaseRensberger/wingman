package openai

import (
	"github.com/chaserensberger/wingman/models/providers"
)

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        "openai",
		Name:      "OpenAI",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
