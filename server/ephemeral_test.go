package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func setupEphemeralServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := New(Config{Store: nil})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

func setupMemoryServer(t *testing.T) (*httptest.Server, store.Store) {
	t.Helper()
	st := memory.NewStore()
	srv := New(Config{Store: st})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, st
}

func assert501(t *testing.T, resp *http.Response, wantMsg string) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode 501 body: %v", err)
	}
	if body.Error != wantMsg {
		t.Errorf("expected error %q, got %q", wantMsg, body.Error)
	}
}

func TestEphemeralServer_CRUDReturns501(t *testing.T) {
	ts := setupEphemeralServer(t)
	msg := "persistence is disabled; this server is running in ephemeral mode"

	endpoints := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/sessions", mustJSON(t, map[string]any{"title": "x"})},
		{http.MethodGet, "/sessions", nil},
		{http.MethodPost, "/agents", mustJSON(t, map[string]any{"name": "x"})},
		{http.MethodGet, "/agents", nil},
		{http.MethodPost, "/clients", mustJSON(t, map[string]any{"name": "x"})},
		{http.MethodGet, "/clients", nil},
	}

	for _, ep := range endpoints {
		req, _ := http.NewRequest(ep.method, ts.URL+ep.path, bytes.NewReader(ep.body))
		if ep.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", ep.method, ep.path, err)
		}
		assert501(t, resp, msg)
	}
}

func TestEphemeralServer_PerSessionRunReturns501(t *testing.T) {
	ts := setupEphemeralServer(t)
	hint := "persistence is disabled; use POST /run for ephemeral runs"

	for _, path := range []string{"/sessions/xxx/message", "/sessions/xxx/message/stream"} {
		resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(mustJSON(t, map[string]any{
			"agent_id": "agt_x",
			"message":  "hi",
		})))
		if err != nil {
			t.Fatalf("POST %s failed: %v", path, err)
		}
		assert501(t, resp, hint)
	}
}

func TestEphemeralServer_RunEndpointStreamsWithoutPersistence(t *testing.T) {
	ts := setupEphemeralServer(t)

	model := looptest.NewRecordingModel(looptest.Reply("hello from ephemeral"))
	model.SetInfo(models.ModelInfo{
		Provider:     "test-structured",
		ID:           "fake-model",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	testModelMu.Lock()
	testModelFactory = func() models.Model { return model }
	testModelMu.Unlock()
	defer func() {
		testModelMu.Lock()
		testModelFactory = nil
		testModelMu.Unlock()
	}()

	body := mustJSON(t, map[string]any{
		"agent": map[string]any{
			"name":         "ephemeral-agent",
			"instructions": "You are a test agent.",
			"provider":     "test-structured",
			"model":        "fake-model",
		},
		"message": "hi",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /run failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := makeSSEScanner(resp)
	var foundDone bool
	for scanner.Scan() {
		if scanner.Event().Type == "done" {
			foundDone = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("sse scanner error: %v", err)
	}
	if !foundDone {
		t.Fatal("expected done event in SSE stream")
	}
}

func TestNormalServer_RunEndpointWorksWithAgentID(t *testing.T) {
	ts, _ := setupMemoryServer(t)

	model := looptest.NewRecordingModel(looptest.Reply("hello from agent id"))
	model.SetInfo(models.ModelInfo{
		Provider:     "test-structured",
		ID:           "fake-model",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	testModelMu.Lock()
	testModelFactory = func() models.Model { return model }
	testModelMu.Unlock()
	defer func() {
		testModelMu.Lock()
		testModelFactory = nil
		testModelMu.Unlock()
	}()

	// Create agent via API.
	agentBody := mustJSON(t, map[string]any{
		"name":         "run-agent",
		"instructions": "You are a test agent.",
		"provider":     "test-structured",
		"model":        "fake-model",
	})
	resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(agentBody))
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	var agent store.Agent
	decodeJSON(t, resp, &agent)

	// Run with agent_id.
	runBody := mustJSON(t, map[string]any{
		"agent_id": agent.ID,
		"message":  "hi",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/run", bytes.NewReader(runBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /run failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := makeSSEScanner(resp)
	var foundDone bool
	for scanner.Scan() {
		if scanner.Event().Type == "done" {
			foundDone = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("sse scanner error: %v", err)
	}
	if !foundDone {
		t.Fatal("expected done event in SSE stream")
	}
}

func TestNormalServer_RunEndpointWorksWithInlineAgent(t *testing.T) {
	ts, _ := setupMemoryServer(t)

	model := looptest.NewRecordingModel(looptest.Reply("hello from inline"))
	model.SetInfo(models.ModelInfo{
		Provider:     "test-structured",
		ID:           "fake-model",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	testModelMu.Lock()
	testModelFactory = func() models.Model { return model }
	testModelMu.Unlock()
	defer func() {
		testModelMu.Lock()
		testModelFactory = nil
		testModelMu.Unlock()
	}()

	body := mustJSON(t, map[string]any{
		"agent": map[string]any{
			"name":         "inline-agent",
			"instructions": "You are a test agent.",
			"provider":     "test-structured",
			"model":        "fake-model",
		},
		"message": "hi",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /run failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := makeSSEScanner(resp)
	var foundDone bool
	for scanner.Scan() {
		if scanner.Event().Type == "done" {
			foundDone = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("sse scanner error: %v", err)
	}
	if !foundDone {
		t.Fatal("expected done event in SSE stream")
	}
}

func TestEphemeralServer_RunEndpointRejectsAgentID(t *testing.T) {
	ts := setupEphemeralServer(t)

	body := mustJSON(t, map[string]any{
		"agent_id": "agt_xxx",
		"message":  "hi",
	})
	resp, err := http.Post(ts.URL+"/run", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /run failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	want := "agent_id is not supported in ephemeral mode; provide an inline agent spec"
	if errResp.Error != want {
		t.Errorf("expected error %q, got %q", want, errResp.Error)
	}
}
