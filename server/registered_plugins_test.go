package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/execution"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/pluginhub"
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
			contributions := protocol.Contributions{Methods: []protocol.Method{{Name: "echo", InputSchema: map[string]any{"type": "object"}}}}
			config, _ := request.Params.Plugin["config"].(map[string]any)
			if config["action"] == true {
				contributions.Actions = []protocol.Action{{ID: id + ".action", Command: id + "-action"}}
			}
			if config["conflict"] == "boundary" {
				contributions.Parts = []string{id + ".boundary"}
				contributions.ContextBoundary = id + ".boundary"
			}
			if config["conflict"] == "action" {
				contributions.Actions = []protocol.Action{{ID: id + ".compact", Command: "compact"}}
			}
			if count, ok := config["models"].(float64); ok {
				inventory := protocol.Provider{Meta: provider.ProviderMeta{ID: "inventory"}, Models: map[string]models.ModelInfo{}}
				for i := 0; i < int(count); i++ {
					id := fmt.Sprintf("model-%d", i)
					inventory.Models[id] = models.ModelInfo{ID: id, Provider: "inventory"}
				}
				contributions.Providers = []protocol.Provider{inventory}
			}
			result = pluginhost.InitializeResult{
				ProtocolVersion: pluginhost.ProtocolVersion,
				Plugin:          pluginhost.InitializedPlugin{ID: id, Name: id},
				Contributions:   contributions,
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
	binary := filepath.Join(t.TempDir(), "project-context")
	command := exec.Command("go", "build", "-o", binary, "../examples/plugins/projectcontext")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(artifact)
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
			state := pluginhost.State{Root: t.TempDir()}
			if err := state.Trust(t.Context(), project, true); err != nil {
				t.Fatal(err)
			}
			scopes, err := execution.NewManager(execution.Config{Providers: providers, PluginDirs: []string{global}, PluginState: state})
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
				PluginState: state,
				Plugins: []PluginRegistration{
					{ID: "compaction", Enabled: true, Factory: func() plugin.Plugin { factoryCalls++; return compaction.New() }},
					{ID: "wingcode", Enabled: true, Factory: func() plugin.Plugin { factoryCalls++; return wingcode.New() }},
					{ID: "optional", Enabled: false, Factory: func() plugin.Plugin { factoryCalls++; return sessionActionTestPlugin{} }},
				},
			}
			var releases pluginhub.Catalog
			catalog := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/plugin" {
					_, _ = w.Write(artifact)
					return
				}
				_ = json.NewEncoder(w).Encode(releases)
			}))
			defer catalog.Close()
			releases = pluginhub.Catalog{Version: 1, Packages: []pluginhub.Package{{ID: "example/project-context", Name: "Project context", Releases: []pluginhub.Release{{Version: "v1.0.0", Protocol: protocol.Version, Artifacts: map[string]pluginhub.Artifact{runtime.GOOS + "/" + runtime.GOARCH: {URL: catalog.URL + "/plugin", SHA256: hex.EncodeToString(sum[:])}}}}}}}
			cfg.PluginInstaller = &pluginhub.Installer{Root: global, CatalogURL: catalog.URL, Client: catalog.Client(), Update: scopes.UpdatePlugins}
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
			request := func(method, path string, auth bool, want int, payload ...any) []byte {
				t.Helper()
				var input io.Reader
				if len(payload) > 0 {
					data, err := json.Marshal(payload[0])
					if err != nil {
						t.Fatal(err)
					}
					input = bytes.NewReader(data)
				}
				req, err := http.NewRequestWithContext(t.Context(), method, target+path, input)
				if err != nil {
					t.Fatal(err)
				}
				if auth {
					req.SetBasicAuth("test", "recovery-test-password")
				}
				req.Header.Set("Content-Type", "application/json")
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
			for _, path := range []string{"/plugins", "/rpc-plugins", "/rpc-plugins/packages", "/rpc-plugins/catalog"} {
				request(http.MethodGet, path, false, unauthenticated)
			}
			for _, path := range []string{"/rpc-plugins/manage", "/rpc-plugins/trust", "/rpc-plugins/call"} {
				want := http.StatusUnauthorized
				if workflow == "vite-development" {
					want = http.StatusBadRequest
				}
				request(http.MethodPost, path, false, want)
				request(http.MethodPost, path, true, http.StatusBadRequest)
			}
			request(http.MethodPost, "/rpc-plugins/reload", false, unauthenticated)
			for _, query := range []struct {
				value string
				count int
			}{{"project", 1}, {"missing", 0}} {
				var found []pluginhub.Package
				if err := json.Unmarshal(request(http.MethodGet, "/rpc-plugins/catalog?query="+query.value, true, http.StatusOK), &found); err != nil || len(found) != query.count {
					t.Fatalf("search %q = %#v, %v", query.value, found, err)
				}
			}
			beforeInventory := factoryCalls
			var inventory api.PluginsResponse
			if err := json.Unmarshal(request(http.MethodGet, "/plugins", true, http.StatusOK), &inventory); err != nil {
				t.Fatal(err)
			}
			want := []api.RegisteredPlugin{
				{ID: "compaction", Enabled: true}, {ID: "wingcode", Enabled: true},
				{ID: "optional", Enabled: false}, {ID: "orientation", Enabled: true},
			}
			if !reflect.DeepEqual(inventory.Plugins, want) || factoryCalls != beforeInventory {
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
			for _, operation := range []string{"install", "update"} {
				request(http.MethodPost, "/rpc-plugins/manage", true, http.StatusOK, pluginPackageRequest{Operation: operation, Source: "example/project-context"})
			}
			var installed []pluginhub.Installation
			if err := json.Unmarshal(request(http.MethodGet, "/rpc-plugins/packages", true, http.StatusOK), &installed); err != nil || len(installed) != 1 || installed[0].ID != "example/project-context" {
				t.Fatalf("installed packages = %#v, %v", installed, err)
			}
			request(http.MethodPost, "/rpc-plugins/call", true, http.StatusOK, pluginCallRequest{Plugin: "global", Method: "echo", Input: json.RawMessage(`{}`)})
			trustedDirectory := t.TempDir()
			for _, trusted := range []bool{true, false} {
				request(http.MethodPost, "/rpc-plugins/trust", true, http.StatusOK, pluginTrustRequest{Directory: trustedDirectory, Trusted: trusted})
				if got, err := state.Trusted(trustedDirectory); err != nil || got != trusted {
					t.Fatalf("project trust = %v, %v", got, err)
				}
			}
			request(http.MethodPost, "/rpc-plugins/manage", true, http.StatusOK, pluginPackageRequest{Operation: "remove", Source: "example/project-context"})
			actionProject := t.TempDir()
			actionDir := pluginhost.LocalPluginDir(actionProject)
			if err := os.MkdirAll(actionDir, 0o700); err != nil {
				t.Fatal(err)
			}
			actionManifest, _ := json.Marshal(pluginhost.Manifest{ID: "project", Command: []string{executable, "-test.run=^TestSessionPluginProcess$"}, Config: map[string]any{"action": true}})
			if err := os.WriteFile(filepath.Join(actionDir, "wingman-plugin.json"), actionManifest, 0o600); err != nil {
				t.Fatal(err)
			}
			request(http.MethodPost, "/rpc-plugins/trust", true, http.StatusOK, pluginTrustRequest{Directory: actionProject, Trusted: true})
			for _, query := range []struct {
				path    string
				project bool
			}{{"/actions", false}, {"/actions?working_directory=" + url.QueryEscape(actionProject), true}} {
				var response api.ActionsResponse
				if err := json.Unmarshal(request(http.MethodGet, query.path, true, http.StatusOK), &response); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, action := range response.Actions {
					if action.ID == "project.action" {
						found = true
					}
				}
				if found != query.project {
					t.Fatalf("project action in %s = %v", query.path, found)
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
