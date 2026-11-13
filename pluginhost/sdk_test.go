package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	sdk "github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

func TestNativePluginHelper(t *testing.T) {
	if os.Getenv("WINGMAN_NATIVE_PLUGIN_HELPER") != "1" {
		return
	}
	err := sdk.Serve(context.Background(), sdk.Definition{ID: "test/native", Version: "v1.0.0", Setup: func(ctx context.Context, host *sdk.Host, config map[string]any) (*sdk.Extension, error) {
		marker, _ := config["marker"].(string)
		object := map[string]any{"type": "object"}
		var workers sync.WaitGroup
		return &sdk.Extension{
			Parts:           []string{"test/native.note"},
			ContextBoundary: "test/native.note",
			Close:           func(context.Context) error { workers.Wait(); return nil },
			Hooks: run.Hooks{TransformSystem: func(ctx context.Context, info run.TransformSystemInfo) (string, error) {
				var client string
				if err := host.Call(ctx, "identity", nil, &client); err != nil {
					return "", err
				}
				return info.System + "|" + marker + "|" + client, nil
			}, OnTurnStart: func(ctx context.Context, step int) error {
				return host.StorageSet(ctx, "turn_start", step)
			}, OnTurnEnd: func(ctx context.Context, step int, turn run.Turn) error {
				return host.StorageSet(ctx, "turn_end", step)
			}, AfterRun: func(ctx context.Context, info run.AfterRunInfo) error {
				return host.StorageSet(ctx, "steps", info.Result.Steps)
			}},
			Actions: []sdk.Action{{Action: protocol.Action{ID: "test/native.note", Command: "note", InputSchema: json.RawMessage(`{"type":"object"}`)}, Execute: func(ctx context.Context, info agentplugin.ActionInfo) error {
				message, err := info.Client.Generate(ctx, models.Request{Model: info.Model, System: "action"})
				if err != nil {
					return err
				}
				message.Content = append(message.Content, models.OpaquePart{TypeName: "test/native.note", Raw: []byte(`{"type":"test/native.note","value":"note"}`)})
				info.Sink.OnEvent(run.MessageEvent{Message: *message})
				return nil
			}}},
			Tools: []sdk.Tool{{Tool: protocol.Tool{Name: "native_tool", Description: "Native test tool", InputSchema: object}, Execute: func(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
				if inv.Input["cancel_host"] == true {
					child, cancel := context.WithCancel(ctx)
					defer cancel()
					finished := make(chan error, 1)
					go func() { finished <- host.Call(child, "callback.wait", nil, nil) }()
					if err := host.Call(ctx, "callback.started", nil, nil); err != nil {
						return tool.Result{}, err
					}
					cancel()
					if err := <-finished; !errors.Is(err, context.Canceled) {
						return tool.Result{}, fmt.Errorf("child call error = %v", err)
					}
					if err := host.Call(ctx, "callback.await", nil, nil); err != nil {
						return tool.Result{}, err
					}
				}
				if inv.Input["wait"] == true {
					<-ctx.Done()
					return tool.Result{}, ctx.Err()
				}
				inv.Progress.Report("progress", nil)
				var client string
				if err := host.Call(ctx, "identity", nil, &client); err != nil {
					return tool.Result{}, err
				}
				return tool.Result{Text: marker + ":" + client}, nil
			}}},
			Methods: []sdk.Method{{Method: protocol.Method{Name: "empty", InputSchema: object, OutputSchema: map[string]any{"type": "null"}}, Execute: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}, {Method: protocol.Method{Name: "background", InputSchema: object}, Execute: func(ctx context.Context, input json.RawMessage) (any, error) {
				background, err := host.Background(ctx)
				if err != nil {
					return nil, err
				}
				workers.Add(1)
				go func() {
					defer workers.Done()
					// The test releases this callback only after the invocation returns.
					_ = host.Call(background, "background.ready", nil, nil)
					var client string
					if err := host.Call(background, "identity", nil, &client); err == nil {
						_ = host.Call(background, "background.owner", client, nil)
					}
					<-background.Done()
				}()
				return nil, nil
			}}, {Method: protocol.Method{Name: "state", InputSchema: object}, Execute: func(ctx context.Context, input json.RawMessage) (any, error) {
				var value struct {
					Key string `json:"key"`
				}
				if err := json.Unmarshal(input, &value); err != nil {
					return nil, err
				}
				var result json.RawMessage
				err := host.StorageGet(ctx, value.Key, &result)
				return result, err
			}}, {Method: protocol.Method{Name: "forward", InputSchema: object}, Execute: func(ctx context.Context, input json.RawMessage) (any, error) {
				var result json.RawMessage
				err := host.CallPlugin(ctx, "test/native", "remember", input, &result)
				return result, err
			}}, {Method: protocol.Method{Name: "remember", InputSchema: object, OutputSchema: map[string]any{"type": "string"}}, Execute: func(ctx context.Context, input json.RawMessage) (any, error) {
				var value struct {
					Value string `json:"value"`
				}
				if err := json.Unmarshal(input, &value); err != nil {
					return nil, err
				}
				if value.Value != "" {
					if err := host.StorageSet(ctx, "value", value.Value); err != nil {
						return nil, err
					}
				}
				var stored string
				if err := host.StorageGet(ctx, "value", &stored); err != nil {
					return nil, err
				}
				return stored, nil
			}}},
			Providers: []sdk.Provider{{Provider: protocol.Provider{Meta: provider.ProviderMeta{ID: "native", Name: "Native"}, Models: map[string]models.ModelInfo{"echo": {ID: "echo", Provider: "native"}}}, Prepare: func(ctx context.Context, call protocol.ProviderCall) (*models.PreparedRequest, error) {
				if call.Request.System == "prepare-error" {
					return nil, &models.ProviderError{Category: models.ErrorInvalidRequest, Provider: "native", RequestID: "req-prepare", Message: "preparation failed"}
				}
				return &models.PreparedRequest{Model: call.Request.Model, API: "plugin", URL: "plugin://native", Body: map[string]any{"system": call.Request.System, "credential": call.Credential.Key}}, nil
			}, Stream: func(ctx context.Context, call protocol.ProviderCall, emit func(models.StreamPart) error) (*models.Message, error) {
				text := call.Request.System
				if text == "wait-start" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if text == "no-parts" {
					return &models.Message{Role: models.RoleAssistant, FinishReason: models.FinishReasonStop}, nil
				}
				if text == "startup-error" || text == "stream-error" || text == "overflow" {
					var attempt int
					if err := host.Call(ctx, "provider.attempt", nil, &attempt); err != nil {
						return nil, err
					}
					if attempt == 1 {
						if text == "stream-error" {
							if err := emit(models.StreamStartPart{}); err != nil {
								return nil, err
							}
						}
						delay := time.Nanosecond
						failure := &models.ProviderError{Category: models.ErrorRateLimit, Provider: "native", Status: 429, RequestID: "req-native", Retryable: true, RetryAfter: &delay, Message: "rate limited", Metadata: map[string]string{"code": "throttled"}}
						if text == "overflow" {
							failure.Category, failure.Classification, failure.Status, failure.Retryable = models.ErrorInvalidRequest, "context-overflow", 400, false
						}
						return nil, failure
					}
					text = "recovered"
				}
				if text == "credential" {
					text = call.Credential.Key
				}
				if text == "ordered" {
					for i := 0; i < 100; i++ {
						if err := emit(models.TextDeltaPart{ID: "text", Delta: fmt.Sprint(i)}); err != nil {
							return nil, err
						}
					}
				}
				if text == "flood" {
					background, err := host.Background(ctx)
					if err != nil {
						return nil, err
					}
					defer host.Call(background, "flood.finished", nil, nil)
					for i := 0; i < 10000; i++ {
						if err := emit(models.TextDeltaPart{ID: "text", Delta: "data"}); err != nil {
							return nil, err
						}
					}
				}
				for _, part := range []models.StreamPart{models.StreamStartPart{}, models.TextStartPart{ID: "text"}, models.TextDeltaPart{ID: "text", Delta: text}, models.TextEndPart{ID: "text"}} {
					if err := emit(part); err != nil {
						return nil, err
					}
				}
				message := &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: text}}, FinishReason: models.FinishReasonStop}
				if err := emit(models.FinishPart{Message: message, Reason: models.FinishReasonStop}); err != nil {
					return nil, err
				}
				return message, nil
			}}},
		}, nil
	}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func nativeManifest(t *testing.T, dir, marker string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "test/native", Command: []string{executable, "-test.run=^TestNativePluginHelper$"}, Config: map[string]any{"marker": marker}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "wingman-plugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func nativeManager(t *testing.T) (*Manager, string) {
	t.Helper()
	t.Setenv("WINGMAN_NATIVE_PLUGIN_HELPER", "1")
	dir := t.TempDir()
	nativeManifest(t, dir, "old")
	m, err := New(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	state := State{Root: t.TempDir()}
	m.SetHostHandler(func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		if method == "identity" {
			return Identity(ctx).ClientID, nil
		}
		if method == "credential.get" {
			return nil, nil
		}
		return state.Storage(ctx, PluginID(ctx), method, raw)
	})
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m, dir
}

func TestNativeSDKToolHooksProviderAndClientIsolation(t *testing.T) {
	m, _ := nativeManager(t)
	var wg sync.WaitGroup
	for _, client := range []string{"client-a", "client-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			activation, err := m.Acquire(protocol.Identity{ClientID: client})
			if err != nil {
				t.Error(err)
				return
			}
			defer activation.Close(context.Background())
			result, err := activation.Tools()[0].Execute(t.Context(), tool.Invocation{Input: map[string]any{}})
			if err != nil || result.Text != "old:"+client {
				t.Errorf("tool result = %#v, %v", result, err)
				return
			}
			var stored json.RawMessage
			input, _ := json.Marshal(map[string]string{"value": client})
			if err := activation.Invoke(t.Context(), "test/native", "remember", input, &stored); err != nil || string(stored) != `"`+client+`"` {
				t.Errorf("remember = %s, %v", stored, err)
				return
			}
			registry, err := provider.NewRegistry(nil)
			if err != nil {
				t.Error(err)
				return
			}
			registry, err = registry.WithRegistrations(activation.Providers())
			if err != nil {
				t.Error(err)
				return
			}
			sess := session.New(session.WithClient(registry.NewClient(nil)), session.WithModelRef(models.ModelRef{Provider: "native", ID: "echo"}, models.ModelInfo{}), session.WithSystem("base"), session.WithCurrentDate(false), session.WithPlugin(activation.Plugins()...))
			defer sess.Close(context.Background())
			runResult, err := sess.Run(t.Context(), "hello")
			if err != nil {
				t.Error(err)
				return
			}
			if !strings.Contains(runResult.Response, "base|old|"+client) {
				t.Errorf("hook did not reach provider: %#v", runResult)
			}
			for _, key := range []string{"turn_start", "turn_end", "steps"} {
				input, _ := json.Marshal(map[string]string{"key": key})
				var output json.RawMessage
				if err := activation.Invoke(t.Context(), "test/native", "state", input, &output); err != nil || string(output) != "1" {
					t.Errorf("%s = %s, %v", key, output, err)
				}
			}
			if err := sess.RunAction(t.Context(), "test/native.note", json.RawMessage(`{}`), nil); err != nil {
				t.Error(err)
				return
			}
			history := sess.History()
			last := history[len(history)-1]
			if len(last.Content) != 2 || last.Content[1].Type() != "test/native.note" {
				t.Errorf("action did not persist custom part: %#v", last)
			}
		}()
	}
	wg.Wait()
}

func TestNativeGenerationRemainsCallableBetweenCallsAfterReload(t *testing.T) {
	m, dir := nativeManager(t)
	old, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close(context.Background())
	oldPID := old.plugins[0].client.PID()
	nativeManifest(t, dir, "new")
	if err := m.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := old.Tools()[0].Execute(t.Context(), tool.Invocation{})
	if err != nil || result.Text != "old:client" {
		t.Fatalf("old generation = %#v, %v", result, err)
	}
	next, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(context.Background())
	if next.plugins[0].client.PID() == oldPID {
		t.Fatal("reload reused old process")
	}
	result, err = next.Tools()[0].Execute(t.Context(), tool.Invocation{})
	if err != nil || result.Text != "new:client" {
		t.Fatalf("new generation = %#v, %v", result, err)
	}
	var forwarded json.RawMessage
	if err := old.Invoke(t.Context(), "test/native", "forward", json.RawMessage(`{"value":"captured"}`), &forwarded); err != nil || string(forwarded) != `"captured"` {
		t.Fatalf("captured plugin call = %s, %v", forwarded, err)
	}
}

func TestNativeCancellationDoesNotStopSiblingCall(t *testing.T) {
	m, _ := nativeManager(t)
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := a.Tools()[0].Execute(ctx, tool.Invocation{Input: map[string]any{"wait": true}}); err == nil {
		t.Fatal("cancelled call succeeded")
	}
	if _, err := a.Tools()[0].Execute(t.Context(), tool.Invocation{}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeProviderCancellationWithUnreadStream(t *testing.T) {
	m, _ := nativeManager(t)
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.WithRegistrations(a.Providers())
	if err != nil {
		t.Fatal(err)
	}
	client := registry.NewClient(nil)
	ctx, cancel := context.WithCancel(t.Context())
	stream, err := client.Stream(ctx, models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: "flood"})
	if err != nil {
		t.Fatal(err)
	}
	<-stream.Iter()
	cancel()
	for range stream.Iter() {
	}
	if _, err := stream.Final(); err == nil {
		t.Fatal("cancelled stream succeeded")
	}
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := a.Tools()[0].Execute(ctx, tool.Invocation{}); err != nil {
		t.Fatalf("cancelled provider blocked the process: %v", err)
	}
}

func TestNativeDisabledAuthSkipsPluginCredentialFallback(t *testing.T) {
	m, _ := nativeManager(t)
	lookups := 0
	m.SetHostHandler(func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "credential.get" {
			lookups++
			return provider.Credential{Type: "api_key", Key: "plugin-secret"}, nil
		}
		return nil, nil
	})
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(t.Context())
	for _, disabled := range []bool{true, false} {
		auth := !disabled
		registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{"native": {Options: provider.ProviderOptions{Auth: &auth}}})
		if err != nil {
			t.Fatal(err)
		}
		registry, err = registry.WithRegistrations(a.Providers())
		if err != nil {
			t.Fatal(err)
		}
		client := registry.NewClient(nil)
		request := models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: "credential"}
		prepared, err := client.Prepare(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if disabled && (prepared.Body["credential"] != "" || lookups != 0) {
			t.Fatalf("disabled Prepare used credentials: %#v", prepared)
		}
		message, err := client.Generate(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		text := message.Content[0].(models.TextPart).Text
		if disabled && (lookups != 0 || text != "") {
			t.Fatalf("disabled authentication used credentials: %q, lookups=%d", text, lookups)
		}
		if !disabled && (lookups != 2 || text != "plugin-secret") {
			t.Fatalf("enabled credential fallback = %q, lookups=%d", text, lookups)
		}
	}
}

func TestNativeProviderDeliversQueuedPartsBeforeCompletion(t *testing.T) {
	m, _ := nativeManager(t)
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(t.Context())
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.WithRegistrations(a.Providers())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := registry.NewClient(nil).Stream(t.Context(), models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: "ordered"})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	for part := range stream.Iter() {
		if delta, ok := part.(models.TextDeltaPart); ok {
			deltas = append(deltas, delta.Delta)
		}
	}
	if _, err := stream.Final(); err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 101 {
		t.Fatalf("delivered %d deltas, want 101", len(deltas))
	}
	for i := 0; i < 100; i++ {
		if deltas[i] != fmt.Sprint(i) {
			t.Fatalf("delta %d = %q", i, deltas[i])
		}
	}
	if deltas[100] != "ordered" {
		t.Fatalf("last delta = %q", deltas[100])
	}
}

func TestNativeUnreadProviderDoesNotBlockOtherCalls(t *testing.T) {
	m, _ := nativeManager(t)
	floodFinished := make(chan struct{})
	m.SetHostHandler(func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "identity" {
			return Identity(ctx).ClientID, nil
		}
		if method == "flood.finished" {
			close(floodFinished)
		}
		return nil, nil
	})
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(t.Context())
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.WithRegistrations(a.Providers())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := registry.NewClient(nil).Stream(t.Context(), models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: "flood"})
	if err != nil {
		t.Fatal(err)
	}
	// Waiting for a response on the same connection must not require draining the stream.
	select {
	case <-floodFinished:
	case <-time.After(2 * time.Second):
		t.Fatal("unread stream blocked the RPC connection")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result, err := a.Tools()[0].Execute(ctx, tool.Invocation{})
	if err != nil || result.Text != "old:client" {
		t.Fatalf("sibling call = %#v, %v", result, err)
	}
	for range stream.Iter() {
	}
	_, err = stream.Final()
	if err == nil || !strings.Contains(err.Error(), "consumer is too slow") {
		t.Fatalf("unread stream error = %v", err)
	}
	if _, err := a.Tools()[0].Execute(ctx, tool.Invocation{}); err != nil {
		t.Fatalf("overflow stopped plugin: %v", err)
	}
}

func TestNativeRPCRejectsUndeclaredAndInvalidInput(t *testing.T) {
	m, _ := nativeManager(t)
	a, err := m.Acquire(protocol.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	var output json.RawMessage
	for _, input := range []struct{ name, raw string }{{"missing", `{}`}, {"remember", `[]`}} {
		if err := a.Invoke(t.Context(), "test/native", input.name, json.RawMessage(input.raw), &output); err == nil {
			t.Fatalf("accepted %s %s", input.name, input.raw)
		}
	}
	if err := a.Invoke(t.Context(), "test/native", "empty", json.RawMessage(`{}`), &output); err != nil || string(output) != "null" {
		t.Fatalf("nullable result = %s, %v", output, err)
	}
	generation, err := agentplugin.ActivateAll(a.Plugins()...)
	if err != nil {
		t.Fatal(err)
	}
	defer generation.Close(context.Background())
	part, err := generation.Parts().UnmarshalPart([]byte(`{"type":"test/native.note","value":"persisted"}`))
	if err != nil || part.Type() != "test/native.note" {
		t.Fatalf("custom part = %#v, %v", part, err)
	}
}

func TestNativeBackgroundRetainsOwnerAndStopsBeforeClose(t *testing.T) {
	m, _ := nativeManager(t)
	ready := make(chan struct{})
	release := make(chan struct{})
	owner := make(chan string, 1)
	m.SetHostHandler(func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		switch method {
		case "background.ready":
			close(ready)
			select {
			case <-release:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case "identity":
			return Identity(ctx).ClientID, nil
		case "background.owner":
			var client string
			if err := json.Unmarshal(input, &client); err != nil {
				return nil, err
			}
			owner <- client
		}
		return nil, nil
	})
	a, err := m.Acquire(protocol.Identity{ClientID: "background-client"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var result json.RawMessage
	if err := a.Invoke(ctx, "test/native", "background", json.RawMessage(`{}`), &result); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = a.Close(t.Context())
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("background worker did not start")
	}
	close(release)
	select {
	case client := <-owner:
		if client != "background-client" {
			t.Fatalf("background owner = %s", client)
		}
	case <-time.After(time.Second):
		t.Fatal("background scope expired with invocation")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("background worker prevented shutdown: %v", err)
	}
}

func TestTrustedProjectOverridesGlobalPluginAndCanDisableIt(t *testing.T) {
	t.Setenv("WINGMAN_NATIVE_PLUGIN_HELPER", "1")
	root := t.TempDir()
	global := filepath.Join(root, "z-global")
	project := filepath.Join(root, "a-project", ".wingman", "plugins")
	for _, dir := range []string{global, project} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	nativeManifest(t, global, "global")
	nativeManifest(t, project, "project")
	m, err := New(context.Background(), []string{global, project})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.SetHostHandler(func(ctx context.Context, method string, input json.RawMessage) (any, error) { return "client", nil })
	result, err := m.Tools()[0].Execute(t.Context(), tool.Invocation{})
	if err != nil || result.Text != "project:client" {
		t.Fatalf("project override = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(project, "wingman-plugin.json"), []byte(`{"id":"test/native","disabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(m.Tools()) != 0 {
		t.Fatal("project override did not disable global plugin")
	}
}

func TestNativeProviderPreservesStartupAndEstablishedFailures(t *testing.T) {
	for _, system := range []string{"startup-error", "stream-error"} {
		t.Run(system, func(t *testing.T) {
			m, _ := nativeManager(t)
			m.SetHostHandler(func(context.Context, string, json.RawMessage) (any, error) { return 1, nil })
			a, err := m.Acquire(protocol.Identity{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close(t.Context())
			base, _ := provider.NewRegistry(nil)
			registry, err := base.WithRegistrations(a.Providers())
			if err != nil {
				t.Fatal(err)
			}
			stream, err := registry.NewClient(nil).Stream(t.Context(), models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: system})
			if system == "stream-error" {
				if err != nil || stream == nil {
					t.Fatalf("established stream returned %v", err)
				}
				_, err = stream.Drain()
			} else if stream != nil {
				t.Fatal("startup rejection returned a stream")
			}
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Category != models.ErrorRateLimit || failure.Status != 429 || failure.RequestID != "req-native" || !failure.Retryable || failure.RetryAfter == nil || *failure.RetryAfter != time.Nanosecond || failure.Metadata["code"] != "throttled" {
				t.Fatalf("provider failure = %#v, %v", failure, err)
			}
			_, err = registry.NewClient(nil).Prepare(t.Context(), models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}, System: "prepare-error"})
			if !errors.As(err, &failure) || failure.RequestID != "req-prepare" {
				t.Fatalf("prepare failure = %#v, %v", failure, err)
			}
		})
	}
}

func TestNativeProviderRetryAndOverflowRecovery(t *testing.T) {
	for _, system := range []string{"startup-error", "stream-error", "overflow"} {
		t.Run(system, func(t *testing.T) {
			m, _ := nativeManager(t)
			var calls atomic.Int32
			m.SetHostHandler(func(context.Context, string, json.RawMessage) (any, error) { return calls.Add(1), nil })
			a, err := m.Acquire(protocol.Identity{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close(t.Context())
			base, _ := provider.NewRegistry(nil)
			registry, err := base.WithRegistrations(a.Providers())
			if err != nil {
				t.Fatal(err)
			}
			recoveries := 0
			result, err := run.Run(t.Context(), run.Config{
				Client: registry.NewClient(nil), Model: models.ModelRef{Provider: "native", ID: "echo"}, System: system,
				Messages: []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original"}}}},
				Retry:    run.RetryPolicy{MaxAttempts: 2, InitialDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
				Hooks: run.Hooks{TransformHistory: func(ctx context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
					if !info.ContextOverflow {
						return nil, nil
					}
					recoveries++
					return []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "summary"}}}}, nil
				}},
			})
			if system == "stream-error" {
				if err == nil || calls.Load() != 1 {
					t.Fatalf("established failure retried: calls=%d err=%v", calls.Load(), err)
				}
				return
			}
			if err != nil || calls.Load() != 2 {
				t.Fatalf("recovery failed: calls=%d err=%v", calls.Load(), err)
			}
			if system == "overflow" && recoveries != 1 {
				t.Fatalf("overflow recovery calls = %d", recoveries)
			}
			if system == "startup-error" && (recoveries != 0 || len(result.Turns) != 1 || result.Turns[0].Attempt != 2) {
				t.Fatalf("startup retry = %#v, recoveries=%d", result, recoveries)
			}
		})
	}
}

func TestNativeHostCallChildCancellationLeavesParentAndSiblingUsable(t *testing.T) {
	m, _ := nativeManager(t)
	started := make(chan struct{})
	stopped := make(chan struct{})
	m.SetHostHandler(func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		switch method {
		case "callback.wait":
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		case "callback.started":
			select {
			case <-started:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case "callback.await":
			select {
			case <-stopped:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case "identity":
			return Identity(ctx).ClientID, nil
		}
		return nil, nil
	})
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(t.Context())
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result, err := a.Tools()[0].Execute(ctx, tool.Invocation{Input: map[string]any{"cancel_host": true}})
	if err != nil || result.Text != "old:client" {
		t.Fatalf("parent call = %#v, %v", result, err)
	}
	if _, err := a.Tools()[0].Execute(ctx, tool.Invocation{}); err != nil {
		t.Fatalf("sibling call failed: %v", err)
	}
}

func TestNativeProviderStartupCancellationAndEmptyCompletion(t *testing.T) {
	m, _ := nativeManager(t)
	a, err := m.Acquire(protocol.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(t.Context())
	base, _ := provider.NewRegistry(nil)
	registry, err := base.WithRegistrations(a.Providers())
	if err != nil {
		t.Fatal(err)
	}
	client := registry.NewClient(nil)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	ref := models.ModelRef{Provider: "native", ID: "echo"}
	stream, err := client.Stream(ctx, models.Request{Model: ref, System: "wait-start"})
	if stream != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled startup = %#v, %v", stream, err)
	}
	message, err := client.Generate(t.Context(), models.Request{Model: ref, System: "no-parts"})
	if err != nil || message == nil || message.Role != models.RoleAssistant {
		t.Fatalf("empty completion = %#v, %v", message, err)
	}
}
