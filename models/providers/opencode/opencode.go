package opencode

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for OpenCode model refs.
const ID = "opencode"

// Model returns an OpenCode model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "OpenCode",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
