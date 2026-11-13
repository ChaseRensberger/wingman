package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/execution"
	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	sdk "github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/plugins/orientation"
	"github.com/chaserensberger/wingman/skill"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/chaserensberger/wingman/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParityPluginProcess(t *testing.T) {
	if os.Getenv("WINGMAN_PARITY_PLUGIN") != "1" {
		return
	}
	err := sdk.Serve(context.Background(), sdk.Definition{ID: "test/parity", Version: "v1.0.0", Setup: func(ctx context.Context, host *sdk.Host, config map[string]any) (*sdk.Extension, error) {
		var prompts atomic.Int64
		var authWaiting atomic.Bool
		marker, _ := config["marker"].(string)
		transforms := &sdk.Transforms{}
		transforms.MCP(func(_ context.Context, servers protocol.MCPServers) error {
			if url, _ := config["mcp_url"].(string); url != "" {
				servers["policy"] = wingmcp.ServerConfig{Type: "remote", URL: url}
			}
			return nil
		})
		registration := transforms.Agents(func(_ context.Context, agents protocol.Agents) error {
			for id, a := range agents {
				a.Instructions += "|policy:" + marker
				agents[id] = a
			}
			if _, exists := agents["virtual"]; !exists {
				agents["virtual"] = store.Agent{ID: "virtual", Name: "Virtual", ModelRef: "parity/echo", Instructions: "virtual|" + marker}
			}
			return nil
		})
		transforms.Skills(func(_ context.Context, skills protocol.Skills) error {
			skills["policy"] = skill.Skill{ID: "policy", Name: "Policy", Description: "Policy skill", Content: marker}
			return nil
		})
		transforms.References(func(_ context.Context, refs protocol.References) error {
			refs["policy"] = orientation.Reference{Name: "policy", Path: "/policy", Description: marker}
			return nil
		})
		transforms.Models(func(_ context.Context, models protocol.Models) error {
			for id, entry := range models["parity"] {
				entry.MaxOutput = 123
				models["parity"][id] = entry
			}
			return nil
		})
		return &sdk.Extension{
			Transforms: transforms,
			Actions:    []sdk.Action{{Action: protocol.Action{ID: "test/parity.capture", Command: "capture", InputSchema: json.RawMessage(`{"type":"object"}`)}, Execute: func(context.Context, agentplugin.ActionInfo) error { return nil }}},
			OnPrompt: func(ctx context.Context, input protocol.Prompt) (string, error) {
				prompts.Add(1)
				if input.Message == "remember-scope" {
					if err := host.StorageSet(ctx, "prepared", true); err != nil {
						return "", err
					}
				}
				if input.Message == "reject" {
					return "", fmt.Errorf("rejected")
				}
				return input.Message + "|" + marker, nil
			},
			Authentication: []sdk.Authentication{{AuthMethod: protocol.AuthMethod{ID: "signin", Integration: "parity", Label: "Test sign-in", Kind: "oauth"}, Handle: func(ctx context.Context, input protocol.AuthCall) (protocol.AuthResult, error) {
				if string(input.Input) == `{"wait":true}` {
					authWaiting.Store(true)
					defer authWaiting.Store(false)
					<-ctx.Done()
					return protocol.AuthResult{}, ctx.Err()
				}
				if input.Operation == "cancel" {
					return protocol.AuthResult{Status: "cancelled"}, nil
				}
				if input.Operation == "complete" {
					if string(input.State) != `{"private":"secret"}` {
						return protocol.AuthResult{}, fmt.Errorf("private state lost")
					}
					return protocol.AuthResult{Status: "connected", Credential: &provider.Credential{Type: "api_key", Key: "secret-" + marker}}, nil
				}
				return protocol.AuthResult{Status: "pending", URL: "https://example.com/signin", Instructions: "Use test sign-in", State: json.RawMessage(`{"private":"secret"}`)}, nil
			}}},
			Methods: []sdk.Method{
				{Method: protocol.Method{Name: "prepared", InputSchema: map[string]any{"type": "object"}}, Execute: func(ctx context.Context, _ json.RawMessage) (any, error) {
					var value bool
					err := host.StorageGet(ctx, "prepared", &value)
					return value, err
				}},
				{Method: protocol.Method{Name: "auth_waiting", InputSchema: map[string]any{"type": "object"}}, Execute: func(context.Context, json.RawMessage) (any, error) { return authWaiting.Load(), nil }},
				{Method: protocol.Method{Name: "subscribe", InputSchema: map[string]any{"type": "object"}}, Execute: func(ctx context.Context, _ json.RawMessage) (any, error) {
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()
					sub, err := host.SubscribeEvents(ctx, "plugin.test/parity.changed")
					if err != nil {
						return nil, err
					}
					defer sub.Close()
					if err := host.PublishEvent(ctx, "changed", map[string]string{"value": marker}); err != nil {
						return nil, err
					}
					events, err := sub.Next()
					cancel()
					if err != nil {
						return nil, err
					}
					return events, sub.Close()
				}},
				{Method: protocol.Method{Name: "count", InputSchema: map[string]any{"type": "object"}}, Execute: func(context.Context, json.RawMessage) (any, error) { return prompts.Load(), nil }},
				{Method: protocol.Method{Name: "dispose", InputSchema: map[string]any{"type": "object"}}, Execute: func(context.Context, json.RawMessage) (any, error) { registration.Dispose(); return nil, nil }},
				{Method: protocol.Method{Name: "emit", InputSchema: map[string]any{"type": "object"}}, Execute: func(ctx context.Context, raw json.RawMessage) (any, error) {
					return nil, host.PublishEvent(ctx, "changed", map[string]string{"value": marker})
				}},
			},
			Providers: []sdk.Provider{{Provider: protocol.Provider{Meta: provider.ProviderMeta{ID: "parity", Name: "Parity"}, Models: map[string]models.ModelInfo{"echo": {ID: "echo", Provider: "parity"}}}, Stream: func(ctx context.Context, call protocol.ProviderCall, emit func(models.StreamPart) error) (*models.Message, error) {
				return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: call.Request.System}}, FinishReason: models.FinishReasonStop}, nil
			}}},
		}, nil
	}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func parityServer(t *testing.T) (*Server, *execution.Manager, *memory.Store, string, string) {
	t.Helper()
	return parityServerWithConfig(t, Config{})
}

func parityServerWithConfig(t *testing.T, cfg Config) (*Server, *execution.Manager, *memory.Store, string, string) {
	t.Helper()
	t.Setenv("WINGMAN_PARITY_PLUGIN", "1")
	root := t.TempDir()
	parityManifest(t, root, "old")
	providers, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	state := pluginhost.State{Root: t.TempDir()}
	scopes, err := execution.NewManager(execution.Config{Providers: providers, PluginDirs: []string{root}, PluginState: state})
	if err != nil {
		t.Fatal(err)
	}
	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store, cfg.Scopes, cfg.PluginState = data, scopes, state
	s := New(cfg)
	t.Cleanup(func() { _ = s.Close(context.Background()); _ = scopes.Close(); _ = data.Close() })
	return s, scopes, data, client.ID, root
}

func parityManifest(t *testing.T, root, marker string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pluginhost.Manifest{ID: "test/parity", Command: []string{exe, "-test.run=^TestParityPluginProcess$"}, Config: map[string]any{"marker": marker}})
	if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func parityRequest(t *testing.T, s *Server, client, method, path string, input any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("X-Wingman-Client", client)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	return w
}

func TestPluginPromptAdmissionRetryAndFailure(t *testing.T) {
	s, _, data, client, _ := parityServer(t)
	agent := &store.Agent{ID: "agent", Name: "Agent", ModelRef: "parity/echo"}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	sess := &store.Session{ID: "session", ClientID: client}
	if err := data.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	// Keep admitted runs queued so assertions observe admission rather than model execution.
	s.runs.stop()
	input := map[string]any{"agent_id": "agent", "message": "original", "request_id": "request"}
	results := make(chan *httptest.ResponseRecorder, 8)
	for range 8 {
		go func() { results <- parityRequest(t, s, client, "POST", "/sessions/session/message", input) }()
	}
	for range 8 {
		w := <-results
		if w.Code != 202 {
			t.Fatalf("admission: %d %s", w.Code, w.Body)
		}
	}
	admitted, err := data.GetSessionRunByRequestID(context.Background(), sess.ID, "request")
	if err != nil {
		t.Fatal(err)
	}
	if admitted.Message != "original|old" {
		t.Fatalf("message = %q", admitted.Message)
	}
	w := parityRequest(t, s, client, "POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "count", Input: json.RawMessage(`{}`)})
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "1" {
		t.Fatalf("hook reran: %d %s", w.Code, w.Body)
	}
	input["message"] = "different"
	w = parityRequest(t, s, client, "POST", "/sessions/session/message", input)
	if w.Code != 409 {
		t.Fatalf("conflict: %d %s", w.Code, w.Body)
	}
	input["message"], input["request_id"] = "reject", "rejected"
	w = parityRequest(t, s, client, "POST", "/sessions/session/message", input)
	if w.Code != 400 {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}
	runs, err := data.ListSessionRuns(context.Background(), sess.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v %v", runs, err)
	}
}

func TestPluginAuthRetainsGenerationAndHidesCredentials(t *testing.T) {
	s, scopes, _, client, root := parityServer(t)
	w := parityRequest(t, s, client, "GET", "/rpc-plugins/auth", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "signin") {
		t.Fatalf("methods: %d %s", w.Code, w.Body)
	}
	w = parityRequest(t, s, client, "POST", "/rpc-plugins/auth", pluginAuthRequest{Plugin: "test/parity", Method: "signin", Operation: "start"})
	var status pluginAuthStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.Status != "pending" || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	parityManifest(t, root, "new")
	if err := scopes.ReloadPlugins(context.Background()); err != nil {
		t.Fatal(err)
	}
	w = parityRequest(t, s, client, "POST", "/rpc-plugins/auth", pluginAuthRequest{Attempt: status.ID, Operation: "complete"})
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("complete: %d %s", w.Code, w.Body)
	}
	var credential provider.Credential
	raw, _ := json.Marshal(map[string]string{"key": "parity"})
	value, err := s.pluginState.Credential(pluginhost.WithIdentity(context.Background(), protocol.Identity{ClientID: client}), "test/parity", "credential.get", raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	if err := json.Unmarshal(encoded, &credential); err != nil {
		t.Fatal(err)
	}
	if credential.Key != "secret-old" {
		t.Fatalf("credential generation = %q", credential.Key)
	}
}

func TestPluginEventsAreClientScopedAndBounded(t *testing.T) {
	bus := &pluginEventBus{subs: map[string]*pluginEventSubscription{}}
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	value, err := bus.call(context.Background(), lifetime, "a", "plugin", "events.subscribe", json.RawMessage(`{"types":["custom"]}`))
	if err != nil {
		t.Fatal(err)
	}
	id := value.(string)
	raw, _ := json.Marshal(pluginEventRequest{ID: id})
	if _, err := bus.call(context.Background(), lifetime, "b", "plugin", "events.next", raw); err == nil {
		t.Fatal("cross-client access")
	}
	bus.publish(protocol.Event{Identity: protocol.Identity{ClientID: "b"}, Type: "custom", Data: json.RawMessage(`{}`)})
	bus.publish(protocol.Event{Identity: protocol.Identity{ClientID: "a"}, Type: "other", Data: json.RawMessage(`{}`)})
	bus.publish(protocol.Event{Identity: protocol.Identity{ClientID: "a"}, Type: "custom", Data: json.RawMessage(`{"ok":true}`)})
	result, err := bus.call(context.Background(), lifetime, "a", "plugin", "events.next", raw)
	if err != nil || len(result.([]protocol.Event)) != 1 {
		t.Fatalf("events = %v, %v", result, err)
	}
	for range 129 {
		bus.publish(protocol.Event{Identity: protocol.Identity{ClientID: "a"}, Type: "custom", Data: json.RawMessage(`{}`)})
	}
	if _, err := bus.call(context.Background(), lifetime, "a", "plugin", "events.next", raw); err == nil {
		t.Fatal("overflow silently lost events")
	}
	bus.mu.Lock()
	remaining := len(bus.subs)
	bus.mu.Unlock()
	if remaining != 0 {
		t.Fatal("overflow subscription retained")
	}
	value, err = bus.call(context.Background(), lifetime, "a", "plugin", "events.subscribe", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	bus.mu.Lock()
	sub := bus.subs[value.(string)]
	bus.mu.Unlock()
	bus.shutdown()
	select {
	case <-sub.done:
	default:
		t.Fatal("shutdown left subscription open")
	}
	if _, err := bus.call(context.Background(), lifetime, "a", "plugin", "events.subscribe", json.RawMessage(`{}`)); err == nil {
		t.Fatal("shutdown accepted a subscription")
	}
}

func TestPluginPublishesCustomEvents(t *testing.T) {
	s, _, _, client, _ := parityServer(t)
	w := parityRequest(t, s, client, "POST", "/rpc-plugins/events", pluginEventRequest{Operation: "subscribe", Types: []string{"plugin.test/parity.changed"}})
	var sub pluginEventResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &sub) != nil {
		t.Fatalf("subscribe: %d %s", w.Code, w.Body)
	}
	w = parityRequest(t, s, client, "POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "emit", Input: json.RawMessage(`{}`)})
	if w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	w = parityRequest(t, s, client, "POST", "/rpc-plugins/events", pluginEventRequest{Operation: "next", ID: sub.ID})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":"old"`) {
		t.Fatalf("events: %d %s", w.Code, w.Body)
	}
	w = parityRequest(t, s, client, "POST", "/rpc-plugins/events", pluginEventRequest{Operation: "close", ID: sub.ID})
	if w.Code != 200 {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}
}

func TestPluginTransformsCaptureAndDispose(t *testing.T) {
	s, _, data, client, _ := parityServer(t)
	agent := &store.Agent{ID: "agent", Name: "Agent", Instructions: "base", ModelRef: "parity/echo"}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Wingman-Client", client)
	a, release, err := s.requestPluginActivation(r, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	captured, err := s.transformAgent(context.Background(), a, agent, false)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Instructions != "base|policy:old" {
		t.Fatal(captured.Instructions)
	}
	var ignored json.RawMessage
	if err := a.Invoke(context.Background(), "test/parity", "dispose", json.RawMessage(`{}`), &ignored); err != nil {
		t.Fatal(err)
	}
	next, err := s.transformAgent(context.Background(), a, agent, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.Instructions != "base" || captured.Instructions != "base|policy:old" || agent.Instructions != "base" {
		t.Fatal("transform disposal mutated captured or base agent")
	}
	registry, err := a.ProviderRegistry(context.Background(), s.providers)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := registry.Catalog().Get("parity", "echo")
	if !ok || model.MaxOutput != 123 {
		t.Fatalf("model = %+v", model)
	}
	w := parityRequest(t, s, client, "GET", "/provider/parity/models", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"max_output":123`) {
		t.Fatalf("discovery: %d %s", w.Code, w.Body)
	}
}

func TestPluginAuthCancellationAndShutdown(t *testing.T) {
	for _, operation := range []string{"cancel", "shutdown"} {
		t.Run(operation, func(t *testing.T) {
			s, _, data, client, _ := parityServer(t)
			w := parityRequest(t, s, client, "POST", "/rpc-plugins/auth", pluginAuthRequest{Plugin: "test/parity", Method: "signin", Operation: "start"})
			var status pluginAuthStatus
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil {
				t.Fatalf("start: %d %s", w.Code, w.Body)
			}
			other, err := data.CreateClient("Other")
			if err != nil {
				t.Fatal(err)
			}
			w = parityRequest(t, s, other.ID, "POST", "/rpc-plugins/auth", pluginAuthRequest{Attempt: status.ID, Operation: "cancel"})
			if w.Code != 404 {
				t.Fatalf("cross-client cancel: %d %s", w.Code, w.Body)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- parityRequest(t, s, client, "POST", "/rpc-plugins/auth", pluginAuthRequest{Attempt: status.ID, Operation: "status", Input: json.RawMessage(`{"wait":true}`)})
			}()
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for {
				w = parityRequest(t, s, client, "POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "auth_waiting", Input: json.RawMessage(`{}`)})
				if strings.TrimSpace(w.Body.String()) == "true" {
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("authentication did not start")
				case <-tick.C:
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if operation == "shutdown" {
				if err := s.Close(ctx); err != nil {
					t.Fatal(err)
				}
				s.pluginAuthMu.Lock()
				remaining := len(s.pluginAuth)
				s.pluginAuthMu.Unlock()
				if remaining != 0 {
					t.Fatal("shutdown retained authentication attempts")
				}
			} else {
				w = parityRequest(t, s, client, "POST", "/rpc-plugins/auth", pluginAuthRequest{Attempt: status.ID, Operation: "cancel"})
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
					t.Fatalf("cancel: %d %s", w.Code, w.Body)
				}
			}
			select {
			case w := <-done:
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
					t.Fatalf("interrupted status: %d %s", w.Code, w.Body)
				}
			case <-ctx.Done():
				t.Fatal("authentication did not cancel")
			}
		})
	}
}

func TestPluginSDKSubscriptionCancellationReleasesResources(t *testing.T) {
	s, _, _, client, _ := parityServer(t)
	w := parityRequest(t, s, client, "POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "subscribe", Input: json.RawMessage(`{}`)})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":"old"`) {
		t.Fatalf("subscription: %d %s", w.Code, w.Body)
	}
	s.pluginEvents.mu.Lock()
	remaining := len(s.pluginEvents.subs)
	s.pluginEvents.mu.Unlock()
	if remaining != 0 {
		t.Fatal("cancelled SDK subscription retained")
	}
}

func TestPluginTransformsRunAndDiscovery(t *testing.T) {
	s, scopes, _, client, root := parityServer(t)
	w := parityRequest(t, s, client, "GET", "/agents/virtual", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `virtual|old`) {
		t.Fatalf("agent discovery: %d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Wingman-Client", client)
	agent, pluginAgent, err := s.agentForRun(r, "", "virtual")
	if err != nil {
		t.Fatal(err)
	}
	captured, err := s.buildSessionWithStore(context.Background(), agent, &store.Session{ID: "ephemeral", ClientID: client}, nil, "", nil, nil, &sessionDraft{PluginAgent: pluginAgent})
	if err != nil {
		t.Fatal(err)
	}
	defer captured.Close(context.Background())
	parityManifest(t, root, "new")
	if err := scopes.ReloadPlugins(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := captured.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"virtual|old", "Policy skill", "policy", "/policy"} {
		if !strings.Contains(result.Response, expected) {
			t.Fatalf("run omitted %q: %s", expected, result.Response)
		}
	}
	if strings.Contains(result.Response, "virtual|new") {
		t.Fatal("captured run changed generation")
	}
	w = parityRequest(t, s, client, "GET", "/agents/virtual", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `virtual|new`) {
		t.Fatalf("new discovery: %d %s", w.Code, w.Body)
	}
}

func TestPluginAdmissionCapturesAgentAndSkills(t *testing.T) {
	for _, id := range []string{"virtual", "stored"} {
		for _, kind := range []string{"message", "action"} {
			t.Run(id+"/"+kind, func(t *testing.T) {
				s, scopes, data, client, root := parityServer(t)
				s.runs.stop()
				if id == "stored" {
					if err := data.CreateAgent(&store.Agent{ID: id, Name: "Stored", ModelRef: "parity/echo", Instructions: "stored"}); err != nil {
						t.Fatal(err)
					}
				}
				sess := &store.Session{ID: "captured", ClientID: client}
				if err := data.CreateSession(sess); err != nil {
					t.Fatal(err)
				}
				path := "/sessions/captured/message"
				input := map[string]any{"agent_id": id, "message": "hello", "request_id": "once"}
				if kind == "action" {
					path = "/sessions/captured/actions/capture"
					input = map[string]any{"agent_id": id, "input": map[string]any{}, "request_id": "once"}
				}
				w := parityRequest(t, s, client, "POST", path, input)
				if w.Code != 202 {
					t.Fatalf("admission: %d %s", w.Code, w.Body)
				}
				queued, err := data.GetSessionRunByRequestID(t.Context(), sess.ID, "once")
				if err != nil {
					t.Fatal(err)
				}
				if queued.Agent.ModelRef != "parity/echo" || !strings.Contains(queued.Agent.Instructions, "old") || !strings.Contains(queued.EffectiveInstructions, queued.Agent.Instructions) {
					t.Fatalf("uncaptured agent: %+v", queued)
				}
				found := false
				for _, entry := range queued.Skills {
					if entry.ID == "policy" {
						found = entry.Content == "old"
					}
				}
				if !found {
					t.Fatalf("uncaptured skills: %+v", queued.Skills)
				}
				parityManifest(t, root, "new")
				if err := scopes.ReloadPlugins(t.Context()); err != nil {
					t.Fatal(err)
				}
				w = parityRequest(t, s, client, "POST", path, input)
				if w.Code != 202 {
					t.Fatalf("retry after reload: %d %s", w.Code, w.Body)
				}
				if kind == "action" {
					return
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), sess.ID); err != nil {
					t.Fatal(err)
				}
				runtime, err := s.buildSessionForRunWithSkills(t.Context(), &queued.Agent, sess, queued.ID, queued.Skills)
				if err != nil {
					t.Fatal(err)
				}
				defer runtime.Close(context.Background())
				result, err := runtime.Run(t.Context(), queued.Message)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(result.Response, queued.Agent.Instructions) || strings.Contains(result.Response, "policy:new") || strings.Contains(result.Response, "virtual|new") {
					t.Fatalf("run changed agent: %s", result.Response)
				}
			})
		}
	}
}

func TestPluginPromptStorageUsesCanonicalProject(t *testing.T) {
	s, _, _, client, _ := parityServer(t)
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := s.preparePluginPrompt(t.Context(), &store.Session{ID: "session", ClientID: client, WorkDir: alias}, "virtual", "request", "remember-scope"); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{real, alias} {
		w := parityRequest(t, s, client, "POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "prepared", Input: json.RawMessage(`{}`), WorkingDirectory: directory})
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "true" {
			t.Fatalf("storage via %s: %d %s", directory, w.Code, w.Body)
		}
	}
}

func TestPluginParityRequestPaths(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("API reached frontend: %s", r.URL.Path) }))
			defer frontend.Close()
			cfg := Config{Username: "test", Password: "recovery-test-password"}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			s, _, data, client, _ := parityServerWithConfig(t, cfg)
			s.runs.stop()
			if err := data.CreateSession(&store.Session{ID: "session", ClientID: client}); err != nil {
				t.Fatal(err)
			}
			host := httptest.NewServer(s)
			defer host.Close()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, target)
			}
			request := func(method, path string, input, output any) {
				t.Helper()
				raw, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				r, err := http.NewRequestWithContext(t.Context(), method, target+path, bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				r.SetBasicAuth("test", "recovery-test-password")
				r.Header.Set("X-Wingman-Client", client)
				r.Header.Set("Content-Type", "application/json")
				response, err := http.DefaultClient.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, body)
				}
				if output != nil {
					if err := json.Unmarshal(body, output); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, path := range []string{"/agents", "/agents/virtual", "/provider", "/provider/parity", "/provider/parity/models", "/provider/parity/models/echo", "/catalog", "/mcp", "/tools", "/rpc-plugins/auth"} {
				request("GET", path, nil, nil)
			}
			var sub pluginEventResponse
			request("POST", "/rpc-plugins/events", pluginEventRequest{Operation: "subscribe", Types: []string{"session.run.queued", "plugin.test/parity.changed"}}, &sub)
			request("POST", "/sessions/session/message", map[string]string{"agent_id": "virtual", "message": "hello", "request_id": "once"}, nil)
			run, err := data.GetSessionRunByRequestID(t.Context(), "session", "once")
			if err != nil || run.Message != "hello|old" {
				t.Fatalf("prepared prompt: %+v %v", run, err)
			}
			request("POST", "/rpc-plugins/call", pluginCallRequest{Plugin: "test/parity", Method: "emit", Input: json.RawMessage(`{}`)}, nil)
			var events pluginEventResponse
			request("POST", "/rpc-plugins/events", pluginEventRequest{Operation: "next", ID: sub.ID}, &events)
			if len(events.Events) != 2 {
				t.Fatalf("public and custom events: %+v", events)
			}
			request("POST", "/rpc-plugins/events", pluginEventRequest{Operation: "close", ID: sub.ID}, nil)
			var auth pluginAuthStatus
			request("POST", "/rpc-plugins/auth", pluginAuthRequest{Operation: "start", Plugin: "test/parity", Method: "signin"}, &auth)
			request("POST", "/rpc-plugins/auth", pluginAuthRequest{Operation: "complete", Attempt: auth.ID}, &auth)
			if auth.Status != "connected" {
				t.Fatalf("sign-in: %+v", auth)
			}
		})
	}
}

func TestPluginMCPTransformOwnsConnections(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "policy", Version: "1"}, nil)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "echo", Description: "Echo"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "policy"}}}, nil, nil
	})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil))
	defer upstream.Close()
	s, scopes, _, client, root := parityServer(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pluginhost.Manifest{ID: "test/parity", Command: []string{exe, "-test.run=^TestParityPluginProcess$"}, Config: map[string]any{"marker": "old", "mcp_url": upstream.URL}})
	if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := scopes.ReloadPlugins(t.Context()); err != nil {
		t.Fatal(err)
	}
	w := parityRequest(t, s, client, "GET", "/mcp", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"policy"`) || !strings.Contains(w.Body.String(), `"status":"connected"`) {
		t.Fatalf("MCP discovery: %d %s", w.Code, w.Body)
	}
	w = parityRequest(t, s, client, "GET", "/tools", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"server":"policy"`) {
		t.Fatalf("tool discovery: %d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Wingman-Client", client)
	a, release, err := s.requestPluginActivation(r, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	lease, err := scopes.Acquire(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(context.Background())
	manager, closeMCP, err := lease.Scope().TransformedMCP(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMCP(context.Background())
	tools := manager.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	parityManifest(t, root, "new")
	if err := scopes.ReloadPlugins(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := tools[0].Execute(t.Context(), tool.Invocation{Input: map[string]any{}})
	if err != nil || result.Text != "policy" {
		t.Fatalf("captured MCP call = %+v, %v", result, err)
	}
	if err := closeMCP(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := tools[0].Execute(t.Context(), tool.Invocation{Input: map[string]any{}}); err == nil {
		t.Fatal("closed MCP connection remains callable")
	}
	if len(lease.Scope().MCP().Status()) != 0 {
		t.Fatal("transform mutated base configuration")
	}
}
