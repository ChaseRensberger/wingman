package mcp

import "testing"

func TestConfigValidate(t *testing.T) {
	enabled := true
	valid := []Config{
		{},
		{Servers: map[string]ServerConfig{"local": {Type: "local", Command: []string{"server"}, Enabled: &enabled}}},
		{Servers: map[string]ServerConfig{"remote": {Type: "remote", URL: "https://example.test/mcp"}}},
	}
	for _, cfg := range valid {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%#v): %v", cfg, err)
		}
	}
	invalid := []Config{
		{Servers: map[string]ServerConfig{"local": {Type: "local"}}},
		{Servers: map[string]ServerConfig{"remote": {Type: "remote", URL: "relative"}}},
		{Servers: map[string]ServerConfig{"server": {Type: "unknown"}}},
		{Servers: map[string]ServerConfig{"server": {Type: "local", Command: []string{"server"}, DiscoveryTimeout: -1}}},
		{Servers: map[string]ServerConfig{"server": {Type: "local", Command: []string{"server"}, ExecutionTimeout: -1}}},
	}
	for _, cfg := range invalid {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%#v) succeeded", cfg)
		}
	}
}

func TestTimeouts(t *testing.T) {
	if got := discoveryTimeout(ServerConfig{}); got != defaultTimeout {
		t.Fatalf("default discovery timeout = %s, want %s", got, defaultTimeout)
	}
	if got := executionTimeout(ServerConfig{}); got != defaultTimeout {
		t.Fatalf("default execution timeout = %s, want %s", got, defaultTimeout)
	}
	cfg := ServerConfig{DiscoveryTimeout: 10, ExecutionTimeout: 20}
	if got := discoveryTimeout(cfg); got.String() != "10ms" {
		t.Fatalf("discovery timeout = %s, want 10ms", got)
	}
	if got := executionTimeout(cfg); got.String() != "20ms" {
		t.Fatalf("execution timeout = %s, want 20ms", got)
	}
}

func TestCloneConfigOwnsNestedValues(t *testing.T) {
	enabled := true
	cfg := Config{Servers: map[string]ServerConfig{"server": {
		Type: "local", Command: []string{"server"}, Environment: map[string]string{"KEY": "value"},
		Headers: map[string]string{"Authorization": "secret"}, Enabled: &enabled,
	}}}
	clone := cloneConfig(cfg)
	cfg.Servers["server"].Command[0] = "changed"
	cfg.Servers["server"].Environment["KEY"] = "changed"
	cfg.Servers["server"].Headers["Authorization"] = "changed"
	enabled = false
	server := clone.Servers["server"]
	if server.Command[0] != "server" || server.Environment["KEY"] != "value" || server.Headers["Authorization"] != "secret" || !*server.Enabled {
		t.Fatalf("clone changed with source: %#v", server)
	}
}
