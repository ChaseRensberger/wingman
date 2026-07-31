package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/permission"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
		check    func(t *testing.T, cfg Config)
	}{
		{
			name: "defaults when absent",
			check: func(t *testing.T, cfg Config) {
				if !reflect.DeepEqual(cfg, Default()) {
					t.Fatalf("Load() = %#v, want %#v", cfg, Default())
				}
			},
		},
		{
			name: "valid full configuration",
			contents: `{
				"server":{"host":"0.0.0.0","port":8080,"db":"~/wingman.db","log_level":"debug","log_format":"text"},
				"plugins":{"dirs":["~/plugins"]},
				"permissions":{"bash":"ask"},
				"agent_permissions":{"research":{"read":"allow"}},
				"provider":{"custom":{"name":"Custom","options":{"baseURL":"https://example.test","query":{"version":"1"}}}},
				"mcp":{"filesystem":{"type":"local","command":["mcp-filesystem"],"cwd":"~/project","environment":{"HOME":"/tmp"}}}
			}`,
			check: func(t *testing.T, cfg Config) {
				if cfg.Server.Port != 8080 {
					t.Fatalf("decoded config = %#v", cfg)
				}
				if got := cfg.Provider["custom"].Options.BaseURL; got != "https://example.test" {
					t.Fatalf("provider base URL = %q", got)
				}
				if got := cfg.MCP["filesystem"].Command[0]; got != "mcp-filesystem" {
					t.Fatalf("MCP command = %q", got)
				}
			},
		},
		{name: "unknown top level field", contents: `{"unknown":true}`, wantErr: "unknown field"},
		{name: "removed model default", contents: `{"models":{"default":"openai/gpt-5"}}`, wantErr: "unknown field"},
		{name: "unknown nested field", contents: `{"server":{"unknown":true}}`, wantErr: "unknown field"},
		{name: "unknown provider field", contents: `{"provider":{"custom":{"unknown":true}}}`, wantErr: "unknown field"},
		{name: "trailing JSON value", contents: `{} {}`, wantErr: "trailing JSON value"},
		{name: "invalid port", contents: `{"server":{"port":0}}`, wantErr: "server.port"},
		{name: "invalid log level", contents: `{"server":{"log_level":"trace"}}`, wantErr: "server.log_level"},
		{name: "invalid log format", contents: `{"server":{"log_format":"pretty"}}`, wantErr: "server.log_format"},
		{name: "empty plugin directory", contents: `{"plugins":{"dirs":[""]}}`, wantErr: "plugins.dirs[0]"},
		{name: "empty provider key", contents: `{"provider":{"":{}}}`, wantErr: "provider has an empty key"},
		{name: "empty agent permission key", contents: `{"agent_permissions":{" ":"allow"}}`, wantErr: "agent_permissions has an empty key"},
		{name: "empty MCP key", contents: `{"mcp":{"":{}}}`, wantErr: "mcp has an empty key"},
		{name: "invalid MCP type", contents: `{"mcp":{"bad":{"type":"stdio","command":["bad"]}}}`, wantErr: "type must be local or remote"},
		{name: "missing local MCP command", contents: `{"mcp":{"bad":{"type":"local"}}}`, wantErr: "local command is required"},
		{name: "invalid remote MCP URL", contents: `{"mcp":{"bad":{"type":"remote","url":"relative"}}}`, wantErr: "absolute HTTP URL"},
		{name: "unsupported MCP OAuth", contents: `{"mcp":{"remote":{"type":"remote","url":"https://example.test","oauth":{}}}}`, wantErr: "unknown field"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wingman.json")
			if test.contents != "" {
				if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(path)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) || !strings.Contains(err.Error(), path) {
					t.Fatalf("Load() error = %v, want path and %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, cfg)
		})
	}
}

func TestNormalize(t *testing.T) {
	input := Default()
	input.Server.DB = "~/data/wingman.db"
	input.Plugins.Dirs = []string{"~", "~/plugins", "/opt/plugins"}
	input.MCP = map[string]wingmcp.ServerConfig{"local": {Type: "local", Command: []string{"test"}, CWD: "~/project"}}
	input.AgentPermissions = map[string]permission.Ruleset{"agent": {{Action: "read", Resource: "*", Effect: permission.EffectAllow}}}

	got, err := input.Normalize("/home/wingman")
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.DB != "/home/wingman/data/wingman.db" {
		t.Fatalf("DB = %q", got.Server.DB)
	}
	if got.MCP["local"].CWD != "/home/wingman/project" {
		t.Fatalf("MCP cwd = %q", got.MCP["local"].CWD)
	}
	defaults, err := Default().Normalize("/home/wingman")
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Server.DB != "/home/wingman/.local/share/wingman/wingman.db" || DefaultPluginDir("/home/wingman") != "/home/wingman/.config/wingman/plugins" {
		t.Fatalf("default paths = %#v, plugin = %q", defaults.Server, DefaultPluginDir("/home/wingman"))
	}
	wantDirs := []string{"/home/wingman", "/home/wingman/plugins", "/opt/plugins"}
	for i, want := range wantDirs {
		if got.Plugins.Dirs[i] != want {
			t.Fatalf("Dirs[%d] = %q, want %q", i, got.Plugins.Dirs[i], want)
		}
	}
	got.Plugins.Dirs[0] = "changed"
	got.AgentPermissions["agent"][0].Action = "changed"
	if input.Plugins.Dirs[0] != "~" || input.AgentPermissions["agent"][0].Action != "read" {
		t.Fatal("Normalize mutated input")
	}

	for _, path := range []string{"~other/db", "~other"} {
		input.Server.DB = path
		if _, err := input.Normalize("/home/wingman"); err == nil || !strings.Contains(err.Error(), "~user") {
			t.Fatalf("Normalize(%q) error = %v, want ~user error", path, err)
		}
	}

	input.Server.DB = "~"
	if _, err := input.Normalize(""); err == nil || !strings.Contains(err.Error(), "home is required") {
		t.Fatalf("Normalize() error = %v, want missing-home error", err)
	}
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		name string
		env  environment
		want string
	}{
		{
			name: "XDG config home",
			env: environment{
				getenv: func(key string) string {
					if key == "XDG_CONFIG_HOME" {
						return "/xdg"
					}
					return ""
				},
				geteuid: func() int { return 1000 },
			},
			want: "/xdg/wingman/wingman.json",
		},
		{
			name: "home fallback",
			env: environment{
				getenv:      func(string) string { return "" },
				geteuid:     func() int { return 1000 },
				userHomeDir: func() (string, error) { return "/home/wingman", nil },
			},
			want: "/home/wingman/.config/wingman/wingman.json",
		},
		{
			name: "sudo user home overrides XDG",
			env: environment{
				getenv: func(key string) string {
					if key == "SUDO_USER" {
						return "wingman"
					}
					return "/xdg"
				},
				geteuid: func() int { return 0 },
				lookupHome: func(name string) (string, error) {
					if name != "wingman" {
						t.Fatalf("lookup user = %q", name)
					}
					return "/home/wingman", nil
				},
			},
			want: "/home/wingman/.config/wingman/wingman.json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := defaultPath(test.env)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("defaultPath() = %q, want %q", got, test.want)
			}
		})
	}
}
