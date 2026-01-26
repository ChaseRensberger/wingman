package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Model       string                    `json:"model,omitempty"`
	Providers   map[string]ProviderConfig `json:"providers,omitempty"`
	Agents      map[string]AgentConfig    `json:"agents,omitempty"`
	Tools       map[string]bool           `json:"tools,omitempty"`
	Permissions map[string]PermissionRule `json:"permissions,omitempty"`
}

type ProviderConfig struct {
	APIKey      string  `json:"api_key,omitempty"`
	Model       string  `json:"model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	BaseURL     string  `json:"base_url,omitempty"`
}

type AgentConfig struct {
	Name         string         `json:"name,omitempty"`
	Description  string         `json:"description,omitempty"`
	Model        string         `json:"model,omitempty"`
	Instructions string         `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	MaxSteps     int            `json:"max_steps,omitempty"`
	Temperature  float64        `json:"temperature,omitempty"`
	Options      map[string]any `json:"options,omitempty"`
}

type PermissionRule struct {
	Default string            `json:"default,omitempty"`
	Rules   map[string]string `json:"rules,omitempty"`
}

var defaultConfig = Config{
	Model: "anthropic/claude-sonnet-4-20250514",
	Tools: map[string]bool{
		"bash":  true,
		"read":  true,
		"write": true,
		"edit":  true,
		"glob":  true,
		"grep":  true,
	},
}

func Load() (*Config, error) {
	cfg := defaultConfig

	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalPath := filepath.Join(homeDir, ".config", "wingman", "wingman.json")
		if err := loadFile(globalPath, &cfg); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load global config: %w", err)
		}
	}

	projectPaths := []string{
		"wingman.json",
		".wingman/wingman.json",
	}
	for _, path := range projectPaths {
		if err := loadFile(path, &cfg); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load project config: %w", err)
		}
	}

	applyEnvOverrides(&cfg)

	return &cfg, nil
}

func LoadFile(path string) (*Config, error) {
	cfg := defaultConfig
	if err := loadFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, cfg)
}

func applyEnvOverrides(cfg *Config) {
	if model := os.Getenv("WINGMAN_MODEL"); model != "" {
		cfg.Model = model
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		p := cfg.Providers["anthropic"]
		p.APIKey = apiKey
		cfg.Providers["anthropic"] = p
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		p := cfg.Providers["openai"]
		p.APIKey = apiKey
		cfg.Providers["openai"] = p
	}
}

func (c *Config) GetProviderConfig(name string) ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[name]
}

func (c *Config) GetAgentConfig(name string) AgentConfig {
	if c.Agents == nil {
		return AgentConfig{}
	}
	return c.Agents[name]
}

func (c *Config) IsToolEnabled(name string) bool {
	if c.Tools == nil {
		return true
	}
	enabled, exists := c.Tools[name]
	if !exists {
		return true
	}
	return enabled
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
