package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/wingharness/storage"
	_ "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
	_ "github.com/chaserensberger/wingman/wingmodels/providers/ollama"
)

func setupTestServer(t *testing.T) (*httptest.Server, storage.Store) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	srv := New(Config{Store: store})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return ts, store
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal json: %v", err)
	}
	return b
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestHealth(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestAgentsCRUD(t *testing.T) {
	ts, _ := setupTestServer(t)

	var agentID string

	t.Run("create agent", func(t *testing.T) {
		temp := 0.7
		body := mustJSON(t, map[string]any{
			"name":         "test-agent",
			"instructions": "You are a test agent.",
			"tools":        []string{"bash", "read"},
			"provider":     "anthropic",
			"model":        "claude-sonnet-4-20250514",
			"options": map[string]any{
				"max_tokens":  4096,
				"temperature": temp,
			},
		})

		resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		var agent storage.Agent
		decodeJSON(t, resp, &agent)

		if agent.Name != "test-agent" {
			t.Errorf("expected name 'test-agent', got %q", agent.Name)
		}
		if agent.Instructions != "You are a test agent." {
			t.Errorf("unexpected instructions: %q", agent.Instructions)
		}
		if len(agent.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(agent.Tools))
		}
		if agent.Provider != "anthropic" {
			t.Errorf("expected provider 'anthropic', got %q", agent.Provider)
		}
		if agent.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", agent.Model)
		}
		if agent.Options["max_tokens"] != float64(4096) {
			t.Errorf("expected max_tokens 4096, got %v", agent.Options["max_tokens"])
		}
		if agent.ID == "" {
			t.Fatal("expected agent ID to be set")
		}
		if agent.CreatedAt == "" {
			t.Error("expected created_at to be set")
		}

		agentID = agent.ID
	})

	t.Run("create agent missing name", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"instructions": "no name"})
		resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("list agents", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/agents")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var agents []*storage.Agent
		decodeJSON(t, resp, &agents)

		if len(agents) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(agents))
		}
		if agents[0].ID != agentID {
			t.Errorf("expected agent ID %q, got %q", agentID, agents[0].ID)
		}
	})

	t.Run("get agent", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/agents/" + agentID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var agent storage.Agent
		decodeJSON(t, resp, &agent)

		if agent.ID != agentID {
			t.Errorf("expected ID %q, got %q", agentID, agent.ID)
		}
	})

	t.Run("get agent not found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/agents/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("update agent", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
			"name":         "updated-agent",
			"instructions": "Updated instructions.",
		})

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/agents/"+agentID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var agent storage.Agent
		decodeJSON(t, resp, &agent)

		if agent.Name != "updated-agent" {
			t.Errorf("expected name 'updated-agent', got %q", agent.Name)
		}
		if agent.Instructions != "Updated instructions." {
			t.Errorf("expected updated instructions, got %q", agent.Instructions)
		}
	})

	t.Run("update agent not found", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"name": "nope"})
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/agents/nonexistent", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("delete agent", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/agents/"+agentID, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		resp, err = http.Get(ts.URL + "/agents/" + agentID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
		}
	})

	t.Run("delete agent not found", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/agents/nonexistent", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestAgentWithoutProvider(t *testing.T) {
	ts, _ := setupTestServer(t)

	body := mustJSON(t, map[string]any{
		"name":         "minimal-agent",
		"instructions": "No provider set.",
	})

	resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var agent storage.Agent
	decodeJSON(t, resp, &agent)

	if agent.Model != "" {
		t.Errorf("expected empty model, got %q", agent.Model)
	}
	if agent.Provider != "" {
		t.Errorf("expected empty provider, got %q", agent.Provider)
	}
}

func TestSessionsCRUD(t *testing.T) {
	ts, _ := setupTestServer(t)

	var sessionID string

	t.Run("create session", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
			"title":    "my session",
			"work_dir": "/tmp/test",
		})

		resp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		var sess storage.Session
		decodeJSON(t, resp, &sess)

		if sess.ID == "" {
			t.Fatal("expected session ID to be set")
		}
		if sess.Title != "my session" {
			t.Errorf("expected title 'my session', got %q", sess.Title)
		}
		if sess.WorkDir != "/tmp/test" {
			t.Errorf("expected work_dir '/tmp/test', got %q", sess.WorkDir)
		}
		if sess.History == nil {
			t.Error("expected history to be initialized")
		}

		sessionID = sess.ID
	})

	t.Run("create session empty body", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader([]byte("{}")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		var sess storage.Session
		decodeJSON(t, resp, &sess)

		if sess.ID == "" {
			t.Fatal("expected session ID")
		}
		// Empty title in the request should populate the default
		// placeholder so the UI never shows a blank label.
		if sess.Title != "New session" {
			t.Errorf("expected default title 'New session', got %q", sess.Title)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/sessions")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var sessions []*storage.Session
		decodeJSON(t, resp, &sessions)

		if len(sessions) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("get session", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/sessions/" + sessionID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var sess storage.Session
		decodeJSON(t, resp, &sess)

		if sess.ID != sessionID {
			t.Errorf("expected ID %q, got %q", sessionID, sess.ID)
		}
	})

	t.Run("get session not found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/sessions/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("update session", func(t *testing.T) {
		newDir := "/tmp/updated"
		newTitle := "renamed session"
		body := mustJSON(t, map[string]any{
			"title":    newTitle,
			"work_dir": newDir,
		})

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/sessions/"+sessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var sess storage.Session
		decodeJSON(t, resp, &sess)

		if sess.Title != newTitle {
			t.Errorf("expected title %q, got %q", newTitle, sess.Title)
		}
		if sess.WorkDir != newDir {
			t.Errorf("expected work_dir %q, got %q", newDir, sess.WorkDir)
		}
	})

	t.Run("update session title only", func(t *testing.T) {
		// Verify that omitting work_dir doesn't clobber it — pointer
		// fields in UpdateSessionRequest are how partial updates work.
		body := mustJSON(t, map[string]any{"title": "title only"})

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/sessions/"+sessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		var sess storage.Session
		decodeJSON(t, resp, &sess)

		if sess.Title != "title only" {
			t.Errorf("expected title 'title only', got %q", sess.Title)
		}
		if sess.WorkDir != "/tmp/updated" {
			t.Errorf("expected work_dir to be preserved, got %q", sess.WorkDir)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/sessions/"+sessionID, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		resp, err = http.Get(ts.URL + "/sessions/" + sessionID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
		}
	})

	t.Run("delete session not found", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/sessions/nonexistent", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestMessageSessionValidation(t *testing.T) {
	ts, _ := setupTestServer(t)

	body := mustJSON(t, map[string]any{"work_dir": "/tmp"})
	resp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	var sess storage.Session
	decodeJSON(t, resp, &sess)

	t.Run("missing message", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"agent_id": "some-id"})
		resp, err := http.Post(ts.URL+"/sessions/"+sess.ID+"/message", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing agent_id", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"message": "hello"})
		resp, err := http.Post(ts.URL+"/sessions/"+sess.ID+"/message", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"message": "hello", "agent_id": "nonexistent"})
		resp, err := http.Post(ts.URL+"/sessions/"+sess.ID+"/message", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("session not found", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"message": "hello", "agent_id": "some-id"})
		resp, err := http.Post(ts.URL+"/sessions/nonexistent/message", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

// TestAbortSession exercises POST /sessions/{id}/abort. We can't easily
// drive a real run from the unit-test harness (no live model), so we
// only verify the surface contract: 404 for unknown sessions, 200 with
// aborted=0 for known sessions with no in-flight runs. The "actually
// cancels a run" path is exercised indirectly by the abortRegistry
// test below.
func TestAbortSession(t *testing.T) {
	ts, _ := setupTestServer(t)

	body := mustJSON(t, map[string]any{"work_dir": "/tmp"})
	resp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	var sess storage.Session
	decodeJSON(t, resp, &sess)

	t.Run("idempotent on idle session", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/sessions/"+sess.ID+"/abort", "application/json", nil)
		if err != nil {
			t.Fatalf("abort failed: %v", err)
		}
		var out AbortSessionResponse
		decodeJSON(t, resp, &out)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if out.SessionID != sess.ID {
			t.Fatalf("session_id mismatch: got %q want %q", out.SessionID, sess.ID)
		}
		if out.Aborted != 0 {
			t.Fatalf("expected aborted=0, got %d", out.Aborted)
		}
	})

	t.Run("404 for unknown session", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/sessions/nonexistent/abort", "application/json", nil)
		if err != nil {
			t.Fatalf("abort failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

// TestAbortRegistry verifies the registration / cancellation core
// directly, since the HTTP surface can't easily simulate an in-flight
// run without a live model.
func TestAbortRegistry(t *testing.T) {
	t.Run("abort cancels registered ctx", func(t *testing.T) {
		reg := newAbortRegistry()
		ctx, release := reg.register("ses_x", t.Context())
		defer release()
		if n := reg.abort("ses_x"); n != 1 {
			t.Fatalf("expected 1 cancellation, got %d", n)
		}
		select {
		case <-ctx.Done():
		default:
			t.Fatal("expected ctx to be cancelled")
		}
	})

	t.Run("multiple registrations all cancel", func(t *testing.T) {
		reg := newAbortRegistry()
		ctxA, releaseA := reg.register("ses_x", t.Context())
		defer releaseA()
		ctxB, releaseB := reg.register("ses_x", t.Context())
		defer releaseB()
		if n := reg.abort("ses_x"); n != 2 {
			t.Fatalf("expected 2 cancellations, got %d", n)
		}
		for i, c := range []context.Context{ctxA, ctxB} {
			select {
			case <-c.Done():
			default:
				t.Fatalf("ctx %d not cancelled", i)
			}
		}
	})

	t.Run("release removes registration", func(t *testing.T) {
		reg := newAbortRegistry()
		_, release := reg.register("ses_x", t.Context())
		release()
		if n := reg.abort("ses_x"); n != 0 {
			t.Fatalf("expected 0 cancellations after release, got %d", n)
		}
	})

	t.Run("unknown session is no-op", func(t *testing.T) {
		reg := newAbortRegistry()
		if n := reg.abort("ses_does_not_exist"); n != 0 {
			t.Fatalf("expected 0, got %d", n)
		}
	})
}

func TestProviders(t *testing.T) {
	ts, _ := setupTestServer(t)

	t.Run("list providers", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/provider")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var providers []map[string]any
		decodeJSON(t, resp, &providers)

		if len(providers) < 2 {
			t.Fatalf("expected at least 2 providers (anthropic, ollama), got %d", len(providers))
		}

		names := make(map[string]bool)
		for _, p := range providers {
			if id, ok := p["id"].(string); ok {
				names[id] = true
			}
		}
		if !names["anthropic"] {
			t.Error("expected anthropic in provider list")
		}
		if !names["ollama"] {
			t.Error("expected ollama in provider list")
		}
	})

	t.Run("get provider", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/provider/anthropic")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var meta map[string]any
		decodeJSON(t, resp, &meta)

		if meta["id"] != "anthropic" {
			t.Errorf("expected id 'anthropic', got %v", meta["id"])
		}
	})

	t.Run("get provider not found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/provider/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestProviderAuth(t *testing.T) {
	ts, _ := setupTestServer(t)

	t.Run("get auth empty", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/provider/auth")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var authResp ProvidersAuthResponse
		decodeJSON(t, resp, &authResp)

		if len(authResp.Providers) != 0 {
			t.Errorf("expected empty providers, got %d", len(authResp.Providers))
		}
	})

	t.Run("set auth", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
			"providers": map[string]any{
				"anthropic": map[string]any{
					"type": "api_key",
					"key":  "sk-test-key",
				},
			},
		})

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/provider/auth", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("get auth after set", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/provider/auth")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var authResp ProvidersAuthResponse
		decodeJSON(t, resp, &authResp)

		info, ok := authResp.Providers["anthropic"]
		if !ok {
			t.Fatal("expected anthropic in auth providers")
		}
		if !info.Configured {
			t.Error("expected anthropic to be configured")
		}
		if info.Type != "api_key" {
			t.Errorf("expected type 'api_key', got %q", info.Type)
		}
	})

	t.Run("set auth unknown provider", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
			"providers": map[string]any{
				"unknown-provider": map[string]any{
					"type": "api_key",
					"key":  "test",
				},
			},
		})

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/provider/auth", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("delete auth", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/provider/auth/anthropic", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		resp, err = http.Get(ts.URL + "/provider/auth")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		var authResp ProvidersAuthResponse
		decodeJSON(t, resp, &authResp)

		if _, ok := authResp.Providers["anthropic"]; ok {
			t.Error("expected anthropic to be removed from auth")
		}
	})

	t.Run("delete auth not configured", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/provider/auth/anthropic", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("delete auth unknown provider", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/provider/auth/unknown-provider", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestAgentProviderRoundtrip(t *testing.T) {
	ts, _ := setupTestServer(t)

	temp := 0.5
	body := mustJSON(t, map[string]any{
		"name":     "provider-test",
		"provider": "anthropic",
		"model":    "claude-sonnet-4-20250514",
		"options": map[string]any{
			"max_tokens":  8192,
			"temperature": temp,
		},
	})

	resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	var created storage.Agent
	decodeJSON(t, resp, &created)

	resp, err = http.Get(ts.URL + "/agents/" + created.ID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	var fetched storage.Agent
	decodeJSON(t, resp, &fetched)

	if fetched.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", fetched.Provider)
	}
	if fetched.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", fetched.Model)
	}
	if fetched.Options["max_tokens"] != float64(8192) {
		t.Errorf("expected max_tokens 8192, got %v", fetched.Options["max_tokens"])
	}
	if fetched.Options["temperature"] != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", fetched.Options["temperature"])
	}
}

