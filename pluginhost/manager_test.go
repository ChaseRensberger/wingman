package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/tool"
)

func TestManagerRPCProcessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_MANAGER_RPC_HELPER") != "1" {
		return
	}
	dec := json.NewDecoder(bufio.NewReader(os.Stdin))
	enc := json.NewEncoder(os.Stdout)
	read := func() map[string]any {
		var request map[string]any
		if err := dec.Decode(&request); err != nil {
			os.Exit(0)
		}
		return request
	}
	respond := func(request map[string]any, result any) {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result})
	}
	init := read()
	params, _ := init["params"].(map[string]any)
	plugin, _ := params["plugin"].(map[string]any)
	config, _ := plugin["config"].(map[string]any)
	scenario, _ := config["scenario"].(string)
	id, _ := plugin["id"].(string)
	if scenario == "bad-protocol" {
		respond(init, map[string]any{"protocol_version": 2, "plugin": map[string]any{"id": id}})
		return
	}
	if scenario == "bad-id" {
		respond(init, map[string]any{"protocol_version": 1, "plugin": map[string]any{"id": "other"}})
		return
	}
	if scenario == "unsupported-capability" {
		respond(init, map[string]any{"protocol_version": 1, "plugin": map[string]any{"id": id}, "capabilities": []string{"nope"}})
		return
	}
	name := "authoritative-" + id
	if scenario == "collision" {
		name = "collision"
	}
	capabilities := []string{}
	if scenario == "health" || scenario == "dies" {
		capabilities = []string{CapabilityHealth}
	} else if scenario == "cancel" {
		capabilities = []string{CapabilityCancellation}
	} else if scenario == "progress" {
		capabilities = []string{CapabilityProgress}
	}
	respond(init, map[string]any{
		"protocol_version": 1,
		"plugin":           map[string]any{"id": id, "name": "Initialized " + id, "version": "1.2.3"},
		"capabilities":     capabilities,
		"contributions":    map[string]any{"tools": []any{map[string]any{"name": name, "description": "helper", "input_schema": map[string]any{"type": "object"}}}},
	})
	if scenario == "dies" {
		go func() { time.Sleep(50 * time.Millisecond); os.Exit(3) }()
	}
	for {
		request := read()
		switch request["method"] {
		case HealthMethod:
			respond(request, map[string]any{"status": "ok", "message": "healthy"})
		case "tool.execute":
			if scenario == "cancel" {
				if request := read(); request["method"] != CancelRequestMethod {
					os.Exit(2)
				}
				continue
			}
			if scenario == "progress" {
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": ToolProgressMethod, "params": map[string]any{"request_id": request["id"], "output_delta": "halfway", "metadata": map[string]any{"percent": 50}}})
			}
			if scenario == "slow" {
				time.Sleep(500 * time.Millisecond)
			}
			if scenario == "identity" {
				params, _ := request["params"].(map[string]any)
				invocation, _ := params["context"].(map[string]any)
				respond(request, map[string]any{"text": invocation["session_id"], "metadata": invocation})
				continue
			}
			respond(request, map[string]any{"text": scenario})
		case ShutdownMethod:
			respond(request, map[string]any{"message": "bye"})
			return
		}
	}
}

func TestManagerInitializeUsesAuthoritativeToolsAndProgress(t *testing.T) {
	dir := t.TempDir()
	writeManagerManifest(t, dir, "one", "progress")
	m := newTestManager(t, dir)
	defer m.Close()
	statuses, errs := m.Status()
	if len(errs) != 0 || len(statuses) != 1 || statuses[0].Name != "Initialized one" || statuses[0].PluginVersion != "1.2.3" {
		t.Fatalf("status = %#v, errors = %#v", statuses, errs)
	}
	tools := m.Tools()
	if len(tools) != 1 || tools[0].Name() != "authoritative-one" {
		t.Fatalf("tools = %#v", tools)
	}
	progress := make(chan string, 1)
	result, err := tools[0].Execute(context.Background(), tool.Invocation{Progress: tool.NewProgress(func(delta string, _ map[string]any) { progress <- delta })})
	if err != nil || result.Text != "progress" {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}
	if got := <-progress; got != "halfway" {
		t.Fatalf("progress = %q", got)
	}
}

func TestManagerPassesToolInvocationIdentity(t *testing.T) {
	dir := t.TempDir()
	writeManagerManifest(t, dir, "one", "identity")
	m := newTestManager(t, dir)
	defer m.Close()
	result, err := m.Tools()[0].Execute(context.Background(), tool.Invocation{
		SessionID: "ses_1", RunID: "run_1", AgentID: "agt_1", ToolUseID: "tlu_1", CallID: "call_1", WorkDir: "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "ses_1" || result.Metadata["run_id"] != "run_1" || result.Metadata["tool_use_id"] != "tlu_1" || result.Metadata["work_dir"] != "/tmp/project" {
		t.Fatalf("result = %#v", result)
	}
}

func TestManagerRejectsInvalidCandidatesAndKeepsOldGeneration(t *testing.T) {
	dir := t.TempDir()
	writeManagerManifest(t, dir, "one", "old")
	m := newTestManager(t, dir)
	defer m.Close()
	writeManagerManifest(t, dir, "one", "bad-protocol")
	if err := m.Reload(context.Background()); err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("Reload() error = %v", err)
	}
	result, err := m.Tools()[0].Execute(context.Background(), tool.Invocation{})
	if err != nil || result.Text != "old" {
		t.Fatalf("old tool Execute() = %#v, %v", result, err)
	}
}

func TestManagerToolCancellationPropagatesToPlugin(t *testing.T) {
	dir := t.TempDir()
	writeManagerManifest(t, dir, "one", "cancel")
	m := newTestManager(t, dir)
	defer m.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Tools()[0].Execute(ctx, tool.Invocation{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestManagerRejectsIdentityAndToolCollisions(t *testing.T) {
	for _, tc := range []struct{ name, first, second string }{
		{"identity", "bad-id", ""},
		{"collision", "collision", "collision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManagerManifest(t, dir, "one", tc.first)
			if tc.second != "" {
				writeManagerManifest(t, dir, "two", tc.second)
			}
			_, err := New(context.Background(), []string{dir})
			if err == nil {
				t.Fatal("New() succeeded")
			}
		})
	}
}

func TestManagerReloadSwapsGenerationAndDrainsActiveCall(t *testing.T) {
	dir := t.TempDir()
	writeManagerManifest(t, dir, "one", "slow")
	m := newTestManager(t, dir)
	defer m.Close()
	old := m.Tools()[0]
	finished := make(chan error, 1)
	go func() { _, err := old.Execute(context.Background(), tool.Invocation{}); finished <- err }()
	plugin := old.(*rpcTool).plugin
	deadline := time.Now().Add(time.Second)
	for {
		plugin.mu.Lock()
		active := plugin.activeCalls
		plugin.mu.Unlock()
		if active == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tool call did not acquire its plugin lease")
		}
		time.Sleep(time.Millisecond)
	}
	writeManagerManifest(t, dir, "one", "new")
	reloaded := make(chan error, 1)
	go func() { reloaded <- m.Reload(context.Background()) }()
	select {
	case err := <-reloaded:
		callErr := <-finished
		t.Fatalf("Reload returned before active call drained: reload=%v call=%v", err, callErr)
	case <-time.After(50 * time.Millisecond):
	}
	if err := <-finished; err != nil {
		t.Fatalf("old Execute() error = %v", err)
	}
	if err := <-reloaded; err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if _, err := old.Execute(context.Background(), tool.Invocation{}); err == nil || !strings.Contains(err.Error(), "retiring") {
		t.Fatalf("stale Execute() error = %v", err)
	}
	result, err := m.Tools()[0].Execute(context.Background(), tool.Invocation{})
	if err != nil || result.Text != "new" {
		t.Fatalf("new Execute() = %#v, %v", result, err)
	}
}

func TestManagerHealthExitAndReloadRollback(t *testing.T) {
	global := t.TempDir()
	writeManagerManifest(t, global, "global", "health")
	m := newTestManager(t, global)
	defer m.Close()
	writeManagerManifest(t, global, "local", "bad-id")
	if err := m.Reload(context.Background()); err == nil {
		t.Fatal("Reload() succeeded")
	}
	if len(m.Tools()) != 1 {
		t.Fatalf("tools after rollback = %d", len(m.Tools()))
	}
	writeManagerManifest(t, global, "local", "dies")
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	statuses, _ := m.Status()
	for _, status := range statuses {
		if status.ID == "local" && status.Status != "failed" {
			t.Fatalf("dead plugin status = %#v", status)
		}
	}
}

func newTestManager(t *testing.T, dir string) *Manager {
	t.Helper()
	t.Setenv("GO_WANT_MANAGER_RPC_HELPER", "1")
	m, err := New(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func writeManagerManifest(t *testing.T, dir, id, scenario string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"id": id, "command": []string{os.Args[0], "-test.run=^TestManagerRPCProcessHelper$"}, "config": map[string]any{"scenario": scenario}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".plugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
