// Package config loads and validates the Wingman daemon configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/permission"
)

// Config is the wingman.json daemon configuration.
type Config struct {
	Server           ServerConfig                       `json:"server"`
	Plugins          PluginConfig                       `json:"plugins"`
	Permissions      permission.Ruleset                 `json:"permissions"`
	AgentPermissions map[string]permission.Ruleset      `json:"agent_permissions"`
	Provider         map[string]provider.ProviderConfig `json:"provider"`
	MCP              map[string]wingmcp.ServerConfig    `json:"mcp"`
}

// ServerConfig contains daemon listener, storage, and logging settings.
type ServerConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	DB        string `json:"db"`
	LogLevel  string `json:"log_level"`
	LogFormat string `json:"log_format"`
}

// PluginConfig contains additional global plugin directories.
type PluginConfig struct {
	Dirs       []string `json:"dirs"`
	DefaultDir string   `json:"-"`
}

// Default returns the default daemon configuration.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:      "127.0.0.1",
			Port:      2323,
			LogLevel:  "info",
			LogFormat: "json",
		},
	}
}

// Load reads and validates a configuration file. A missing file returns Default.
func Load(path string) (Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode config %q: trailing JSON value", path)
		}
		return Config{}, fmt.Errorf("decode config %q: trailing JSON value: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

// Validate checks daemon-level configuration values.
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if !oneOf(c.Server.LogLevel, "debug", "info", "warn", "error") {
		return fmt.Errorf("server.log_level must be debug, info, warn, or error")
	}
	if !oneOf(c.Server.LogFormat, "json", "text") {
		return fmt.Errorf("server.log_format must be json or text")
	}
	for i, dir := range c.Plugins.Dirs {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("plugins.dirs[%d] must not be empty", i)
		}
	}
	if err := validateMapKeys("agent_permissions", c.AgentPermissions); err != nil {
		return err
	}
	if err := validateMapKeys("provider", c.Provider); err != nil {
		return err
	}
	if err := validateMapKeys("mcp", c.MCP); err != nil {
		return err
	}
	return (wingmcp.Config{Servers: c.MCP}).Validate()
}

// Normalize returns a copy of c with DB and plugin paths expanded relative to home.
func (c Config) Normalize(home string) (Config, error) {
	out := c
	var err error
	dbPath := c.Server.DB
	if dbPath == "" && home != "" {
		dbPath = filepath.Join(home, ".local", "share", "wingman", "wingman.db")
	}
	if out.Server.DB, err = expandHome(dbPath, home); err != nil {
		return Config{}, fmt.Errorf("normalize server.db: %w", err)
	}
	out.Plugins.Dirs = make([]string, len(c.Plugins.Dirs))
	out.Plugins.DefaultDir = DefaultPluginDir(home)
	for i, dir := range c.Plugins.Dirs {
		if out.Plugins.Dirs[i], err = expandHome(dir, home); err != nil {
			return Config{}, fmt.Errorf("normalize plugins.dirs[%d]: %w", i, err)
		}
	}
	out.Permissions = append(permission.Ruleset(nil), c.Permissions...)
	out.AgentPermissions = cloneAgentPermissions(c.AgentPermissions)
	out.Provider = cloneProviders(c.Provider)
	out.MCP = cloneMCP(c.MCP)
	for name, server := range out.MCP {
		if server.CWD != "" {
			server.CWD, err = expandHome(server.CWD, home)
			if err != nil {
				return Config{}, fmt.Errorf("normalize mcp.%s.cwd: %w", name, err)
			}
			out.MCP[name] = server
		}
	}
	return out, nil
}

// DefaultPluginDir returns the effective user's global plugin directory.
func DefaultPluginDir(home string) string {
	return filepath.Join(home, ".config", "wingman", "plugins")
}

// DefaultPath returns the conventional path for wingman.json.
func DefaultPath() (string, error) {
	return defaultPath(systemEnvironment())
}

// HomeDir returns the effective user's home directory, including sudo-aware
// resolution used by the daemon configuration path.
func HomeDir() (string, error) {
	return homeDir(systemEnvironment())
}

func systemEnvironment() environment {
	return environment{
		getenv:      os.Getenv,
		geteuid:     os.Geteuid,
		userHomeDir: os.UserHomeDir,
		lookupHome: func(name string) (string, error) {
			u, err := user.Lookup(name)
			if err != nil {
				return "", err
			}
			return u.HomeDir, nil
		},
	}
}

type environment struct {
	getenv      func(string) string
	geteuid     func() int
	userHomeDir func() (string, error)
	lookupHome  func(string) (string, error)
}

func defaultPath(env environment) (string, error) {
	sudoUser := env.getenv("SUDO_USER")
	if env.geteuid() != 0 || sudoUser == "" {
		if dir := env.getenv("XDG_CONFIG_HOME"); dir != "" {
			return filepath.Join(dir, "wingman", "wingman.json"), nil
		}
	}

	home, err := homeDir(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wingman", "wingman.json"), nil
}

func homeDir(env environment) (string, error) {
	var home string
	var err error
	sudoUser := env.getenv("SUDO_USER")
	if env.geteuid() == 0 && sudoUser != "" {
		home, err = env.lookupHome(sudoUser)
	} else {
		home, err = env.userHomeDir()
	}
	if err != nil {
		return "", fmt.Errorf("resolve config home: %w", err)
	}
	return home, nil
}

func expandHome(path, home string) (string, error) {
	switch {
	case path == "~":
		if home == "" {
			return "", fmt.Errorf("home is required to expand ~")
		}
		return home, nil
	case strings.HasPrefix(path, "~/"):
		if home == "" {
			return "", fmt.Errorf("home is required to expand ~/")
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	case strings.HasPrefix(path, "~"):
		return "", fmt.Errorf("~user paths are not supported: %q", path)
	default:
		return path, nil
	}
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateMapKeys[V any](name string, values map[string]V) error {
	for key := range values {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%s has an empty key", name)
		}
	}
	return nil
}

func cloneAgentPermissions(in map[string]permission.Ruleset) map[string]permission.Ruleset {
	if in == nil {
		return nil
	}
	out := make(map[string]permission.Ruleset, len(in))
	for key, rules := range in {
		out[key] = append(permission.Ruleset(nil), rules...)
	}
	return out
}

func cloneProviders(in map[string]provider.ProviderConfig) map[string]provider.ProviderConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]provider.ProviderConfig, len(in))
	for key, value := range in {
		value.AuthTypes = append([]provider.AuthType(nil), value.AuthTypes...)
		if value.Options.Query != nil {
			query := make(map[string]string, len(value.Options.Query))
			for queryKey, queryValue := range value.Options.Query {
				query[queryKey] = queryValue
			}
			value.Options.Query = query
		}
		if value.Models != nil {
			models := make(map[string]models.ModelInfo, len(value.Models))
			for modelKey, model := range value.Models {
				model.Env = append([]string(nil), model.Env...)
				models[modelKey] = model
			}
			value.Models = models
		}
		out[key] = value
	}
	return out
}

func cloneMCP(in map[string]wingmcp.ServerConfig) map[string]wingmcp.ServerConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]wingmcp.ServerConfig, len(in))
	for key, value := range in {
		value.Command = append([]string(nil), value.Command...)
		value.Environment = cloneStrings(value.Environment)
		value.Headers = cloneStrings(value.Headers)
		out[key] = value
	}
	return out
}

func cloneStrings(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
