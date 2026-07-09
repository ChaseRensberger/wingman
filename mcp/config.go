// Package mcp adapts configured Model Context Protocol servers into Wingman tools.
package mcp

// Config is the daemon-level MCP configuration loaded from wingman.json.
type Config struct {
	Servers map[string]ServerConfig `json:"servers,omitempty"`
}

// ServerConfig declares one MCP server. Local servers run over stdio; remote
// servers are tried with streamable HTTP first and SSE as a fallback.
type ServerConfig struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	CWD         string            `json:"cwd,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
	Timeout     int               `json:"timeout,omitempty"`
	OAuth       any               `json:"oauth,omitempty"`
}

func (c Config) normalized() Config {
	if c.Servers == nil {
		c.Servers = map[string]ServerConfig{}
	}
	return c
}

func (s ServerConfig) isEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}
