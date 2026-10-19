package api

// RegisteredPlugin describes a plugin bundled with the server.
type RegisteredPlugin struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// PluginsResponse lists the server's build-time plugin registrations.
type PluginsResponse struct {
	Plugins []RegisteredPlugin `json:"plugins"`
}
