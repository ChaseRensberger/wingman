package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/tool"
)

func TestUntrustedProjectDoesNotReadExecutableManifests(t *testing.T) {
	dir := t.TempDir()
	local := pluginhost.LocalPluginDir(dir)
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "wingman-plugin.json"), []byte(`{"id":"invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	providers, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	state := pluginhost.State{Root: t.TempDir()}
	m, err := NewManager(Config{Providers: providers, PluginState: state})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	lease, err := m.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatalf("untrusted manifests affected execution: %v", err)
	}
	defer lease.Close(context.Background())
	if err := state.Trust(t.Context(), dir, true); err != nil {
		t.Fatal(err)
	}
	if err := m.ReloadPlugins(t.Context()); err == nil {
		t.Fatal("trusted invalid executable manifest was accepted")
	}
	if len(lease.Scope().Plugins().Tools()) != 0 {
		t.Fatal("failed trust activation changed current generation")
	}
}

func TestExecutionPluginHelper(t *testing.T) {
	if os.Getenv("WINGMAN_EXECUTION_PLUGIN_HELPER") != "1" {
		return
	}
	err := plugin.Serve(context.Background(), plugin.Definition{ID: "test/project", Version: "v1.0.0", Setup: func(context.Context, *plugin.Host, map[string]any) (*plugin.Extension, error) {
		return &plugin.Extension{Tools: []plugin.Tool{{Tool: protocol.Tool{Name: "project", Description: "Project tool", InputSchema: map[string]any{"type": "object"}}, Execute: func(context.Context, tool.Invocation) (tool.Result, error) { return tool.Result{Text: "project"}, nil }}}}, nil
	}})
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestRevokedTrustBlocksNewActivationWhenReloadFails(t *testing.T) {
	t.Setenv("WINGMAN_EXECUTION_PLUGIN_HELPER", "1")
	dir, global := t.TempDir(), t.TempDir()
	local := pluginhost.LocalPluginDir(dir)
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(pluginhost.Manifest{ID: "test/project", Command: []string{executable, "-test.run=^TestExecutionPluginHelper$"}})
	if err := os.WriteFile(filepath.Join(local, "wingman-plugin.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := pluginhost.State{Root: t.TempDir()}
	if err := state.Trust(t.Context(), dir, true); err != nil {
		t.Fatal(err)
	}
	providers, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Config{Providers: providers, PluginState: state, PluginDirs: []string{global}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	lease, err := m.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(context.Background())
	active, err := lease.Scope().AcquirePlugins(protocol.Identity{ClientID: "client", WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close(context.Background())
	if len(active.Tools()) != 1 {
		t.Fatal("trusted project was not activated")
	}
	if err := state.Trust(t.Context(), dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, "broken.plugin.json"), []byte(`{"id":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.ReloadPlugins(t.Context()); err == nil {
		t.Fatal("invalid global replacement succeeded")
	}
	next, err := lease.Scope().AcquirePlugins(protocol.Identity{ClientID: "client", WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(context.Background())
	if len(next.Tools()) != 0 {
		t.Fatal("new activation retained a revoked project plugin")
	}
	if _, err := active.Tools()[0].Execute(t.Context(), tool.Invocation{}); err != nil {
		t.Fatalf("revocation broke a captured run: %v", err)
	}
}
