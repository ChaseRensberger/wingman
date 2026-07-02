package anthropic

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for Anthropic model refs.
const ID = "anthropic"

// Model returns an Anthropic model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "Anthropic",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
