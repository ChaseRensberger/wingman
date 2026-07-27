package openai

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for OpenAI model refs.
const ID = "openai"

// Model returns an OpenAI model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "OpenAI",
		AuthTypes: []provider.AuthType{{Type: "oauth", Name: "ChatGPT Pro/Plus"}, {Type: "api_key"}},
	})
}
