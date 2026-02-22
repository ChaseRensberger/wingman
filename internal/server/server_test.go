package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "github.com/chaserensberger/wingman/internal/autoregprov"
	"github.com/chaserensberger/wingman/internal/storage"
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
			"provider_id":  "anthropic",
			"provider_options": map[string]any{
				"model":       "claude-sonnet-4-20250514",
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
		if agent.ProviderID != "anthropic" {
			t.Errorf("expected provider_id 'anthropic', got %q", agent.ProviderID)
		}
		if agent.ProviderOptions["model"] != "claude-sonnet-4-20250514" {
			t.Errorf("expected model 'claude-sonnet-4-20250514', got %v", agent.ProviderOptions["model"])
		}
		if agent.ProviderOptions["max_tokens"] != float64(4096) {
			t.Errorf("expected max_tokens 4096, got %v", agent.ProviderOptions["max_tokens"])
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

	if agent.ProviderID != "" {
		t.Errorf("expected empty provider_id, got %q", agent.ProviderID)
	}
}

func TestSessionsCRUD(t *testing.T) {
	ts, _ := setupTestServer(t)

	var sessionID string

	t.Run("create session", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
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
		body := mustJSON(t, map[string]any{
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

		if sess.WorkDir != newDir {
			t.Errorf("expected work_dir %q, got %q", newDir, sess.WorkDir)
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
		"name":        "provider-test",
		"provider_id": "anthropic",
		"provider_options": map[string]any{
			"model":       "claude-sonnet-4-20250514",
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

	if fetched.ProviderID != "anthropic" {
		t.Errorf("expected provider_id 'anthropic', got %q", fetched.ProviderID)
	}
	if fetched.ProviderOptions["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %v", fetched.ProviderOptions["model"])
	}
	if fetched.ProviderOptions["max_tokens"] != float64(8192) {
		t.Errorf("expected max_tokens 8192, got %v", fetched.ProviderOptions["max_tokens"])
	}
	if fetched.ProviderOptions["temperature"] != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", fetched.ProviderOptions["temperature"])
	}
}
