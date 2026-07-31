package deepseek

import (
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for DeepSeek model refs.
const ID = "deepseek"

// Model returns a DeepSeek model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "DeepSeek",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
