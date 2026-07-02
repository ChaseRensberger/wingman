package google

import (
	"github.com/chaserensberger/wingman/models/providers"
)

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        "google",
		Name:      "Google Gemini",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
