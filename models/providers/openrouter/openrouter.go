package openrouter

import (
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for OpenRouter model refs.
const ID = "openrouter"

// Model returns an OpenRouter model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "OpenRouter",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
