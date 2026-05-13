package opencode

import (
	"github.com/chaserensberger/wingman/models/providers"
)

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        "opencode",
		Name:      "OpenCode",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
