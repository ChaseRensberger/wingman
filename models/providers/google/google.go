package google

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for Gemini API model refs.
const ID = "google"

// Model returns a Gemini API model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "Gemini",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
