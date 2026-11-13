package protocol

import (
	"encoding/json"

	"github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugins/orientation"
	"github.com/chaserensberger/wingman/skill"
	"github.com/chaserensberger/wingman/store"
)

// Agents is an owned draft of available agent definitions.
type Agents map[string]store.Agent

// Providers is an owned draft of provider sources before model policy runs.
type Providers map[string]provider.Source

// Models is an owned draft of the models from available providers.
type Models map[string]map[string]models.ModelInfo

// MCPServers is an owned draft of configured MCP servers.
type MCPServers map[string]mcp.ServerConfig

// Skills is an owned draft of available skills.
type Skills map[string]skill.Skill

// References is an owned draft of additional directories advertised to the model.
type References map[string]orientation.Reference

// TransformCall applies one domain's ordered callbacks to a fresh draft.
type TransformCall struct {
	Identity Identity        `json:"context"`
	Domain   string          `json:"domain"`
	Value    json.RawMessage `json:"value"`
}
