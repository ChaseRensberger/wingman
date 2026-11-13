package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/execution"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/plugins/compaction"
)

func TestPluginReloadRejectsBundledConflicts(t *testing.T) {
	t.Setenv("WINGMAN_SESSION_PLUGIN_TEST", "1")
	for _, conflict := range []string{"boundary", "action"} {
		t.Run(conflict, func(t *testing.T) {
			root := t.TempDir()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			write := func(conflict string) {
				t.Helper()
				manifest := pluginhost.Manifest{ID: "test/extension", Command: []string{executable, "-test.run=^TestSessionPluginProcess$"}, Config: map[string]any{"conflict": conflict}}
				data, _ := json.Marshal(manifest)
				if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write("")
			registry, err := provider.NewRegistry(nil)
			if err != nil {
				t.Fatal(err)
			}
			scopes, err := execution.NewManager(execution.Config{Providers: registry, PluginDirs: []string{root}, PluginState: pluginhost.State{Root: t.TempDir()}})
			if err != nil {
				t.Fatal(err)
			}
			defer scopes.Close()
			s := New(Config{Scopes: scopes, Plugins: []PluginRegistration{{ID: "compaction", Enabled: true, Factory: func() agentplugin.Plugin { return compaction.New() }}}})
			defer s.Close(context.Background())
			if err := s.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			lease, err := scopes.Acquire(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close(context.Background())
			before, _ := lease.Scope().Plugins().Status()
			write(conflict)
			response := httptest.NewRecorder()
			s.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rpc-plugins/reload", nil))
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("conflicting reload = %d: %s", response.Code, response.Body.String())
			}
			after, _ := lease.Scope().Plugins().Status()
			if len(after) != 1 || after[0].PID != before[0].PID || !after[0].Running {
				t.Fatalf("rejected reload replaced working generation: %#v", after)
			}
			if err := scopes.ValidatePlugins(t.Context()); err != nil {
				t.Fatalf("working generation no longer activates: %v", err)
			}
			// New project scopes must reject the same conflict instead of publishing it.
			if project, err := scopes.Acquire(t.Context(), t.TempDir()); err == nil {
				_ = project.Close(t.Context())
				t.Fatal("new scope accepted bundled conflict")
			}
			write("")
			if err := scopes.ReloadPlugins(t.Context()); err != nil {
				t.Fatalf("valid replacement failed: %v", err)
			}
		})
	}
}

func BenchmarkListPluginProviderModels(b *testing.B) {
	b.Setenv("WINGMAN_SESSION_PLUGIN_TEST", "1")
	executable, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	root := b.TempDir()
	manifest, _ := json.Marshal(pluginhost.Manifest{ID: "test/inventory", Command: []string{executable, "-test.run=^TestSessionPluginProcess$"}, Config: map[string]any{"models": 1000}})
	if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), manifest, 0o600); err != nil {
		b.Fatal(err)
	}
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		b.Fatal(err)
	}
	scopes, err := execution.NewManager(execution.Config{Providers: registry, PluginDirs: []string{root}})
	if err != nil {
		b.Fatal(err)
	}
	defer scopes.Close()
	s := New(Config{Scopes: scopes})
	defer s.Close(context.Background())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := httptest.NewRecorder()
		s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/provider/inventory/models", nil))
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"model-999"`)) {
			b.Fatalf("model inventory = %d: %s", response.Code, response.Body.String())
		}
	}
	b.StopTimer()
}

func TestPluginStartupRejectsBundledConflict(t *testing.T) {
	t.Setenv("WINGMAN_SESSION_PLUGIN_TEST", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manifest, _ := json.Marshal(pluginhost.Manifest{ID: "test/extension", Command: []string{executable, "-test.run=^TestSessionPluginProcess$"}, Config: map[string]any{"conflict": "boundary"}})
	if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := execution.NewManager(execution.Config{Providers: registry, PluginDirs: []string{root}, PluginState: pluginhost.State{Root: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer scopes.Close()
	s := New(Config{Scopes: scopes, Plugins: []PluginRegistration{{ID: "compaction", Enabled: true, Factory: func() agentplugin.Plugin { return compaction.New() }}}})
	defer s.Close(context.Background())
	if err := s.Start(t.Context()); err == nil || s.ready.Load() {
		t.Fatalf("conflicting startup became ready: %v", err)
	}
}
