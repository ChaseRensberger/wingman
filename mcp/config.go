// Package mcp adapts configured Model Context Protocol servers into Wingman tools.
package mcp

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

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
}

func (c Config) normalized() Config {
	if c.Servers == nil {
		c.Servers = map[string]ServerConfig{}
	}
	return c
}

func cloneConfig(c Config) Config {
	out := Config{Servers: make(map[string]ServerConfig, len(c.Servers))}
	for name, server := range c.Servers {
		server.Command = append([]string(nil), server.Command...)
		server.Environment = cloneStringMap(server.Environment)
		server.Headers = cloneStringMap(server.Headers)
		if server.Enabled != nil {
			enabled := *server.Enabled
			server.Enabled = &enabled
		}
		out.Servers[name] = server
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s ServerConfig) isEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// Validate checks every authored MCP server definition before connections start.
func (c Config) Validate() error {
	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		server := c.Servers[name]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("MCP server name must not be empty")
		}
		if server.Timeout < 0 {
			return fmt.Errorf("MCP server %q timeout must not be negative", name)
		}
		switch server.Type {
		case "local":
			if len(server.Command) == 0 || strings.TrimSpace(server.Command[0]) == "" {
				return fmt.Errorf("MCP server %q local command is required", name)
			}
		case "remote":
			endpoint, err := url.ParseRequestURI(server.URL)
			if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
				return fmt.Errorf("MCP server %q remote URL must be an absolute HTTP URL", name)
			}
		default:
			return fmt.Errorf("MCP server %q type must be local or remote", name)
		}
	}
	return nil
}
