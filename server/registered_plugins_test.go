package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/execution"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/plugins/compaction"
	"github.com/chaserensberger/wingman/plugins/wingcode"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestSessionPluginProcess(t *testing.T) {
	if os.Getenv("WINGMAN_SESSION_PLUGIN_TEST") != "1" {
		return
	}
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	for {
		var request struct {
			ID     json.RawMessage             `json:"id"`
			Method string                      `json:"method"`
			Params pluginhost.InitializeParams `json:"params"`
		}
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		var result any = map[string]any{}
		if request.Method == pluginhost.InitializeMethod {
			id, _ := request.Params.Plugin["id"].(string)
			result = pluginhost.InitializeResult{
				ProtocolVersion: pluginhost.ProtocolVersion,
				Plugin:          pluginhost.InitializedPlugin{ID: id, Name: id},
			}
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			os.Exit(1)
		}
		if request.Method == pluginhost.ShutdownMethod {
			os.Exit(0)
		}
	}
}

func TestRegisteredPluginsRequestPaths(t *testing.T) {
	t.Setenv("WINGMAN_SESSION_PLUGIN_TEST", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	global, project := t.TempDir(), t.TempDir()
	manifest := func(dir, id string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		content, err := json.Marshal(pluginhost.Manifest{ID: id, Command: []string{executable, "-test.run=^TestSessionPluginProcess$"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".plugin.json"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest(global, "global")
	local := pluginhost.LocalPluginDir(project)
	if err := os.MkdirAll(local, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "broken.plugin.json"), []byte(`{"id":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			providers, err := provider.NewRegistry(nil)
			if err != nil {
				t.Fatal(err)
			}
			scopes, err := execution.NewManager(execution.Config{Providers: providers, PluginDirs: []string{global}})
			if err != nil {
				t.Fatal(err)
			}
			defer scopes.Close()
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "broken", WorkDir: project, ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			if lease, err := scopes.Acquire(t.Context(), project); err == nil {
				_ = lease.Close(t.Context())
				t.Fatal("broken project unexpectedly initialized")
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("API reached frontend: %s", r.URL.Path)
			}))
			defer frontend.Close()
			factoryCalls := 0
			cfg := Config{
				Store: data, Scopes: scopes, Username: "test", Password: "recovery-test-password",
				Plugins: []PluginRegistration{
					{ID: "compaction", Enabled: true, Factory: func() plugin.Plugin { factoryCalls++; return compaction.New() }},
					{ID: "wingcode", Enabled: true, Factory: func() plugin.Plugin { factoryCalls++; return wingcode.New() }},
					{ID: "optional", Enabled: false, Factory: func() plugin.Plugin { factoryCalls++; return sessionActionTestPlugin{} }},
				},
			}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			s := New(cfg)
			defer s.Close(context.Background())
			host := httptest.NewServer(s)
			defer host.Close()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			request := func(method, path string, auth bool, want int) []byte {
				t.Helper()
				req, err := http.NewRequestWithContext(t.Context(), method, target+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if auth {
					req.SetBasicAuth("test", "recovery-test-password")
				}
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != want {
					t.Fatalf("%s %s status=%d want=%d: %s", method, path, response.StatusCode, want, body)
				}
				return body
			}
			unauthenticated := http.StatusUnauthorized
			if workflow == "vite-development" {
				unauthenticated = http.StatusOK
			}
			for _, path := range []string{"/plugins", "/rpc-plugins"} {
				request(http.MethodGet, path, false, unauthenticated)
			}
			request(http.MethodPost, "/rpc-plugins/reload", false, unauthenticated)
			var inventory api.PluginsResponse
			if err := json.Unmarshal(request(http.MethodGet, "/plugins", true, http.StatusOK), &inventory); err != nil {
				t.Fatal(err)
			}
			want := []api.RegisteredPlugin{
				{ID: "compaction", Enabled: true}, {ID: "wingcode", Enabled: true},
				{ID: "optional", Enabled: false}, {ID: "orientation", Enabled: true},
			}
			if !reflect.DeepEqual(inventory.Plugins, want) || factoryCalls != 0 {
				t.Fatalf("inventory=%+v factory calls=%d", inventory, factoryCalls)
			}
			if scopes.Count() != 1 {
				t.Fatalf("inventory created a project scope: count=%d", scopes.Count())
			}
			for _, operation := range []struct{ method, path string }{{http.MethodGet, "/rpc-plugins"}, {http.MethodPost, "/rpc-plugins/reload"}} {
				var result struct {
					Plugins []pluginhost.Status `json:"plugins"`
				}
				if err := json.Unmarshal(request(operation.method, operation.path, true, http.StatusOK), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Plugins) != 1 || result.Plugins[0].ID != "global" || result.Plugins[0].Status != "running" {
					t.Fatalf("RPC inventory=%+v", result)
				}
			}
		})
	}
}

func TestRegisteredPluginsWithoutSession(t *testing.T) {
	s := New(Config{})
	defer s.Close(t.Context())
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/plugins", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("ephemeral status=%d: %s", response.Code, response.Body.String())
	}
	var result api.PluginsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Plugins, []api.RegisteredPlugin{{ID: "orientation", Enabled: true}}) {
		t.Fatalf("plugins=%+v", result.Plugins)
	}
}

func TestDisabledRegistrationDoesNotActivate(t *testing.T) {
	s := New(Config{Plugins: []PluginRegistration{{ID: "test", Enabled: false, Factory: func() plugin.Plugin {
		t.Fatal("disabled plugin was constructed")
		return sessionActionTestPlugin{}
	}}}})
	defer s.Close(t.Context())
	if plugins := s.runtimePlugins(); len(plugins) != 1 || plugins[0].Name() != "orientation" {
		t.Fatalf("runtime plugins=%v", plugins)
	}
	actions, err := s.pluginActions()
	if err != nil || len(actions) != 0 {
		t.Fatalf("disabled actions=%v err=%v", actions, err)
	}
}
