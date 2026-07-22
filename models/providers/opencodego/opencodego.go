// Package opencodego registers OpenCode Go model routes.
package opencodego

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

// ID is the provider ID for OpenCode Go model refs.
const ID = "opencode-go"

// Model returns an OpenCode Go model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "OpenCode Go",
		AuthTypes: []provider.AuthType{{Type: "api_key"}},
	})
}
