package anthropic

import (
	"github.com/chaserensberger/wingman/models/providers"
)

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        "anthropic",
		Name:      "Anthropic",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
