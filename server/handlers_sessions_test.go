package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestCreateSessionCommitsAggregateEvent(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	request := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(`{"title":"Event sourced"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var session store.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.Title != "Event sourced" || session.ClientID != client.ID {
		t.Fatalf("session = %#v", session)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != store.EventSessionCreated || events[0].Version != 1 {
		t.Fatalf("events = %#v", events)
	}
	projected, err := store.ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	if projected.ID != session.ID || projected.Title != session.Title || projected.ClientID != session.ClientID {
		t.Fatalf("projected = %#v, response = %#v", projected, session)
	}
}

func TestSessionMetadataCommandsUseExpectedVersion(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	session := &store.Session{ID: "ses_metadata", Title: "Before", ClientID: client.ID}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})

	rename := httptest.NewRequest(http.MethodPost, "/sessions/ses_metadata/rename", strings.NewReader(`{"title":"After","expected_version":1}`))
	rename.Header.Set("Content-Type", "application/json")
	renameResponse := httptest.NewRecorder()
	server.router.ServeHTTP(renameResponse, rename)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status = %d, want %d: %s", renameResponse.Code, http.StatusOK, renameResponse.Body.String())
	}
	var renamed store.Session
	if err := json.NewDecoder(renameResponse.Body).Decode(&renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "After" || renamed.AggregateVersion != 2 {
		t.Fatalf("renamed session = %#v", renamed)
	}

	move := httptest.NewRequest(http.MethodPost, "/sessions/ses_metadata/move", strings.NewReader(`{"working_directory":".","expected_version":1}`))
	move.Header.Set("Content-Type", "application/json")
	moveResponse := httptest.NewRecorder()
	server.router.ServeHTTP(moveResponse, move)
	if moveResponse.Code != http.StatusConflict {
		t.Fatalf("move status = %d, want %d: %s", moveResponse.Code, http.StatusConflict, moveResponse.Body.String())
	}

	legacy := httptest.NewRequest(http.MethodPut, "/sessions/ses_metadata", strings.NewReader(`{"title":"Legacy"}`))
	legacyResponse := httptest.NewRecorder()
	server.router.ServeHTTP(legacyResponse, legacy)
	if legacyResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("legacy update status = %d, want %d", legacyResponse.Code, http.StatusMethodNotAllowed)
	}
}

func TestListSessionModelCalls(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	session := &store.Session{ID: "ses_test", Title: "Test", ClientID: client.ID}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	for _, call := range []store.ModelCall{
		{ID: "mcl_first", SessionID: session.ID, Step: 1, Status: store.ModelCallStatusCompleted, TotalTokens: 42},
		{ID: "mcl_second", SessionID: session.ID, Step: 2, Status: store.ModelCallStatusCompleted, TotalTokens: 84},
	} {
		if err := data.UpsertModelCall(context.Background(), call); err != nil {
			t.Fatal(err)
		}
	}

	server := New(Config{Store: data})
	request := httptest.NewRequest(http.MethodGet, "/sessions/ses_test/model-calls", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var calls []store.ModelCall
	if err := json.NewDecoder(response.Body).Decode(&calls); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].ID != "mcl_first" || calls[1].ID != "mcl_second" {
		t.Fatalf("calls = %#v, want ordered model calls", calls)
	}
}

func TestListSessionModelCallsRejectsOtherClient(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateClient("Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_test", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}

	server := New(Config{Store: data})
	request := httptest.NewRequest(http.MethodGet, "/sessions/ses_test/model-calls", nil)
	request.Header.Set("X-Wingman-Client", other.ID)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestListSessionModelCallsNotFound(t *testing.T) {
	t.Parallel()

	server := New(Config{Store: memory.NewStore()})
	request := httptest.NewRequest(http.MethodGet, "/sessions/ses_missing/model-calls", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestForwardRunEventPublishesToolUpdates(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_test", Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	server := New(Config{Store: data})
	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{
		Data: run.ToolExecutionStartEvent{Call: run.ToolCall{ID: "call_test", Name: "bash", Args: map[string]any{"command": "pwd"}}},
	})
	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{
		Data: run.ToolExecutionEndEvent{Result: run.ToolResult{
			CallID:   "call_test",
			Name:     "bash",
			Args:     map[string]any{"command": "pwd"},
			Output:   "/tmp",
			Metadata: map[string]any{"exit_code": 0},
			Duration: time.Second,
		}},
	})

	events, err := data.ListSessionEvents(context.Background(), "ses_test", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var updates []map[string]any
	for _, event := range events {
		if event.Type != "session.tool.updated" {
			continue
		}
		var update map[string]any
		if err := json.Unmarshal(event.DataJSON, &update); err != nil {
			t.Fatal(err)
		}
		updates = append(updates, update)
	}
	if len(updates) != 2 {
		t.Fatalf("tool updates = %d, want 2", len(updates))
	}
	if updates[0]["status"] != "running" || updates[1]["status"] != "completed" {
		t.Fatalf("tool statuses = %#v", updates)
	}
	if updates[1]["duration_ms"] != float64(1000) {
		t.Fatalf("duration_ms = %#v, want 1000", updates[1]["duration_ms"])
	}
}

func TestForwardRunEventPublishesLiveToolProgress(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_test", Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	server := New(Config{Store: data})
	events, unsubscribe := server.events.subscribe("ses_test")
	defer unsubscribe()

	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{
		Data: run.ToolExecutionProgressEvent{CallID: "call_test", Name: "bash", OutputDelta: "partial"},
	})

	select {
	case event := <-events:
		if event.Type != "session.tool.progress" || event.Seq != 0 {
			t.Fatalf("event = %#v, want live tool progress", event)
		}
		var payload map[string]any
		if err := json.Unmarshal(event.DataJSON, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["run_id"] != "run_test" || payload["call_id"] != "call_test" || payload["output_delta"] != "partial" {
			t.Fatalf("payload = %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tool progress")
	}

	stored, err := data.ListSessionEvents(context.Background(), "ses_test", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("persisted progress events = %d, want 0", len(stored))
	}
}
