package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
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

func TestDeleteSessionPurgesHistoryAndSettlesRuntime(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	session := &store.Session{ID: "ses_delete", ClientID: client.ID}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	live, unsubscribe := server.events.subscribe(session.ID)
	defer unsubscribe()
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	settled := make(chan struct{})
	server.runs.active[session.ID] = cancelWorker
	server.runs.done[session.ID] = settled
	go func() {
		<-workerCtx.Done()
		close(settled)
	}()

	request := httptest.NewRequest(http.MethodDelete, "/sessions/ses_delete?expected_version=1", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if _, err := data.GetSession(session.ID); err == nil {
		t.Fatal("session remains after delete")
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("aggregate events = %d, want 0", len(events))
	}
	select {
	case _, ok := <-live:
		if ok {
			t.Fatal("SSE subscription remains open")
		}
	default:
		t.Fatal("SSE subscription was not closed")
	}
	select {
	case <-settled:
	default:
		t.Fatal("delete returned before runtime settled")
	}
}

func TestDeleteSessionRequiresCurrentVersion(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	session := &store.Session{ID: "ses_delete_version", ClientID: client.ID}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})

	missing := httptest.NewRequest(http.MethodDelete, "/sessions/ses_delete_version", nil)
	missingResponse := httptest.NewRecorder()
	server.router.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusBadRequest {
		t.Fatalf("missing version status = %d, want %d", missingResponse.Code, http.StatusBadRequest)
	}

	stale := httptest.NewRequest(http.MethodDelete, "/sessions/ses_delete_version?expected_version=2", nil)
	staleResponse := httptest.NewRecorder()
	server.router.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale version status = %d, want %d: %s", staleResponse.Code, http.StatusConflict, staleResponse.Body.String())
	}
	if _, err := data.GetSession(session.ID); err != nil {
		t.Fatalf("stale delete mutated session: %v", err)
	}

}

type admissionTestStore struct {
	store.Store
	cancel context.CancelFunc
}

func (s *admissionTestStore) AdmitSessionRun(ctx context.Context, run store.SessionRun) (store.SessionRunAdmission, error) {
	admission, err := s.Store.AdmitSessionRun(ctx, run)
	if err == nil && s.cancel != nil {
		s.cancel()
	}
	return admission, err
}

func (s *admissionTestStore) ClaimNextSessionRun(context.Context, string) (store.SessionRunTransition, error) {
	return store.SessionRunTransition{}, nil
}

func TestMessageSessionAdmissionIsIdempotentAfterRequestCancellation(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_admission_http", ClientID: client.ID, WorkDir: "/snapshot"}); err != nil {
		t.Fatal(err)
	}
	agent := &store.Agent{
		ID:       "agt_admission_http",
		Name:     "Admission",
		ModelRef: "test/model",
		Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
			Provider: "test",
			ID:       "model",
			API:      models.APIOpenAICompatible,
			BaseURL:  "http://127.0.0.1:1",
		}},
	}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	testStore := &admissionTestStore{Store: data, cancel: cancel}
	server := New(Config{Store: testStore})
	body := `{"request_id":"request-1","agent_id":"agt_admission_http","message":"hello"}`

	request := httptest.NewRequest(http.MethodPost, "/sessions/ses_admission_http/message", strings.NewReader(body)).WithContext(requestCtx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	if requestCtx.Err() != context.Canceled {
		t.Fatalf("request context error = %v, want canceled", requestCtx.Err())
	}
	var first MessageSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if first.RunID == "" || first.Status != store.SessionRunStatusQueued || first.SessionVersion != 2 {
		t.Fatalf("first admission = %#v", first)
	}

	testStore.cancel = nil
	retry := httptest.NewRequest(http.MethodPost, "/sessions/ses_admission_http/message", strings.NewReader(body))
	retry.Header.Set("Content-Type", "application/json")
	retryResponse := httptest.NewRecorder()
	server.router.ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, want %d: %s", retryResponse.Code, http.StatusAccepted, retryResponse.Body.String())
	}
	var second MessageSessionResponse
	if err := json.NewDecoder(retryResponse.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.RunID != first.RunID || second.SessionVersion != first.SessionVersion {
		t.Fatalf("retry admission = %#v, first = %#v", second, first)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_admission_http"}, 0, 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("aggregate events = %#v, error = %v", events, err)
	}
	public, err := data.ListSessionEvents(context.Background(), "ses_admission_http", 0, 10)
	if err != nil || len(public) != 1 || public[0].Type != "session.run.queued" {
		t.Fatalf("public events = %#v, error = %v", public, err)
	}

	conflict := httptest.NewRequest(http.MethodPost, "/sessions/ses_admission_http/message", strings.NewReader(`{"request_id":"request-1","agent_id":"agt_admission_http","message":"different"}`))
	conflict.Header.Set("Content-Type", "application/json")
	conflictResponse := httptest.NewRecorder()
	server.router.ServeHTTP(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d: %s", conflictResponse.Code, http.StatusConflict, conflictResponse.Body.String())
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

func TestListSessionToolUses(t *testing.T) {
	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateClient("Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_tools", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	for _, use := range []store.ToolUse{
		{ID: "tlu_second", SessionID: "ses_tools", Name: "write", Status: store.ToolUseStatusProposed, Step: 1, Ordinal: 2, ProposedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), InputJSON: []byte(`{"path":"b"}`), MetadataJSON: []byte(`{"source":"test"}`)},
		{ID: "tlu_first", SessionID: "ses_tools", Name: "read", Status: store.ToolUseStatusProposed, Step: 1, Ordinal: 1, ProposedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), InputJSON: []byte(`{"path":"a"}`), MetadataJSON: []byte(`{"source":"test"}`)},
	} {
		if err := data.SaveToolUse(context.Background(), use); err != nil {
			t.Fatal(err)
		}
	}
	server := New(Config{Store: data})

	request := httptest.NewRequest(http.MethodGet, "/sessions/ses_tools/tool-uses", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var uses []map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&uses); err != nil {
		t.Fatal(err)
	}
	if len(uses) != 2 || string(uses[0]["id"]) != `"tlu_first"` || string(uses[1]["id"]) != `"tlu_second"` {
		t.Fatalf("uses = %#v, want source-ordered tool uses", uses)
	}
	for _, use := range uses {
		for _, field := range []string{"input", "metadata"} {
			var value map[string]any
			if err := json.Unmarshal(use[field], &value); err != nil {
				t.Fatalf("%s = %s, want JSON object: %v", field, use[field], err)
			}
		}
		if _, ok := use["input_json"]; ok {
			t.Fatalf("raw storage field exposed: %#v", use)
		}
		if _, ok := use["metadata_json"]; ok {
			t.Fatalf("raw storage field exposed: %#v", use)
		}
	}

	for _, test := range []struct {
		path   string
		client string
		want   int
	}{
		{"/sessions/ses_tools/tool-uses", other.ID, http.StatusForbidden},
		{"/sessions/ses_missing/tool-uses", "", http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		if test.client != "" {
			request.Header.Set("X-Wingman-Client", test.client)
		}
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.want)
		}
	}
}

func TestForwardRunEventPublishesToolLifecycle(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_test", Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	server := New(Config{Store: data})
	proposedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	call := run.ToolCall{ID: "call_test", ToolUseID: "tlu_test", Name: "bash", Args: map[string]any{"command": "pwd"}, Step: 2, Ordinal: 1, MessageID: "msg_test", PartID: "part_test", ModelCallID: "mcl_test", ProposedAt: proposedAt}
	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{Data: run.ToolUseProposedEvent{Call: call}})
	call.Args = map[string]any{"command": "ls"}
	call.AuthorizedAt = proposedAt.Add(time.Second)
	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{Data: run.ToolUseAuthorizedEvent{Call: call}})
	call.StartedAt = call.AuthorizedAt.Add(time.Second)
	server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{Data: run.ToolExecutionStartEvent{Call: call}})
	for _, status := range []run.ToolUseStatus{run.ToolUseStatusCompleted, run.ToolUseStatusDeclined, run.ToolUseStatusInterrupted} {
		server.forwardRunEvent(context.Background(), "ses_test", "run_test", session.StreamEvent{Data: run.ToolExecutionEndEvent{Result: run.ToolResult{
			CallID: "call_test", ToolUseID: "tlu_test", Status: status, Name: "bash", Args: call.Args, Output: "/tmp", Metadata: map[string]any{"exit_code": 0}, Duration: time.Second,
		}}})
	}

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
	if len(updates) != 6 {
		t.Fatalf("tool updates = %d, want 6", len(updates))
	}
	for _, update := range updates {
		if update["tool_use_id"] != "tlu_test" {
			t.Fatalf("tool use id = %#v, want tlu_test", update)
		}
	}
	if updates[0]["status"] != "proposed" || updates[1]["status"] != "authorized" || updates[2]["status"] != "started" || updates[3]["status"] != "completed" || updates[4]["status"] != "declined" || updates[5]["status"] != "interrupted" {
		t.Fatalf("tool statuses = %#v", updates)
	}
	if updates[1]["input"].(map[string]any)["command"] != "ls" || updates[3]["duration_ms"] != float64(1000) {
		t.Fatalf("tool updates = %#v", updates)
	}
	called := 0
	for _, event := range events {
		if event.Type == "session.tool.called" {
			called++
		}
	}
	if called != 1 {
		t.Fatalf("session.tool.called events = %d, want proposal only", called)
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
		Data: run.ToolExecutionProgressEvent{CallID: "call_test", ToolUseID: "tlu_test", Name: "bash", OutputDelta: "partial"},
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
		if payload["run_id"] != "run_test" || payload["call_id"] != "call_test" || payload["tool_use_id"] != "tlu_test" || payload["output_delta"] != "partial" {
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

type startupRecoveryStore struct {
	store.Store
	order        []string
	interruptErr error
	listErr      error
	resumeErr    error
}

func (s *startupRecoveryStore) InterruptActiveToolUses(context.Context) error {
	s.order = append(s.order, "interrupt")
	if s.interruptErr != nil {
		return s.interruptErr
	}
	return s.Store.InterruptActiveToolUses(context.Background())
}

func (s *startupRecoveryStore) ListRunningSessionRuns(context.Context) ([]store.SessionRun, error) {
	s.order = append(s.order, "list")
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListRunningSessionRuns(context.Background())
}

func (s *startupRecoveryStore) ListQueuedSessionRunSessions(context.Context) ([]string, error) {
	s.order = append(s.order, "resume")
	if s.resumeErr != nil {
		return nil, s.resumeErr
	}
	return nil, nil
}

func TestRecoverStartupInterruptsToolsBeforeRuns(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveToolUse(context.Background(), store.ToolUse{ID: "tlu_recovery", SessionID: "ses_recovery", Name: "bash", Status: store.ToolUseStatusProposed}); err != nil {
		t.Fatal(err)
	}
	recoveryStore := &startupRecoveryStore{Store: data}
	server := New(Config{Store: recoveryStore})
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(recoveryStore.order, ","), "interrupt,list,resume"; got != want {
		t.Fatalf("recovery order = %q, want %q", got, want)
	}
	uses, err := data.ListToolUses(context.Background(), "ses_recovery")
	if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
	}
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	uses, err = data.ListToolUses(context.Background(), "ses_recovery")
	if err != nil || uses[0].Status != store.ToolUseStatusInterrupted {
		t.Fatalf("second recovery tool uses = %#v, error = %v", uses, err)
	}
}

func TestRecoverStartupStopsOnFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		interrupt error
		list      error
		resume    error
		wantOrder string
	}{
		{"interrupt", errors.New("interrupt failed"), nil, nil, "interrupt"},
		{"list", nil, errors.New("list failed"), nil, "interrupt,list"},
		{"resume", nil, nil, errors.New("resume failed"), "interrupt,list,resume"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recoveryStore := &startupRecoveryStore{Store: memory.NewStore(), interruptErr: test.interrupt, listErr: test.list, resumeErr: test.resume}
			server := New(Config{Store: recoveryStore})
			err := server.recoverStartup(context.Background())
			if !errors.Is(err, test.interrupt) && !errors.Is(err, test.list) && !errors.Is(err, test.resume) {
				t.Fatalf("error = %v, want wrapped recovery failure", err)
			}
			if got := strings.Join(recoveryStore.order, ","); got != test.wantOrder {
				t.Fatalf("recovery order = %q, want %q", got, test.wantOrder)
			}
		})
	}
}

func TestRecoverStartupSettlesRunningRunAfterChildState(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_running_recovery"}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_running_recovery", SessionID: "ses_running_recovery", Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(ctx, admission.Run.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, store.StoredMessage{
		ID: "msg_running_recovery", SessionID: admission.Run.SessionID, RunID: admission.Run.ID,
		Role: "assistant", Revision: 1, State: "in_progress",
		Parts: []store.StoredPart{{ID: "part_running_recovery", MessageID: "msg_running_recovery", Kind: "tool", PayloadJSON: []byte(`{"type":"tool","id":"part_running_recovery","tool_use_id":"tlu_running_recovery","call_id":"call","name":"bash","state":"running"}`)}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_running_recovery", SessionID: admission.Run.SessionID, RunID: admission.Run.ID, Step: 1, Status: store.ModelCallStatusStarted, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	use := store.ToolUse{ID: "tlu_running_recovery", SessionID: admission.Run.SessionID, RunID: admission.Run.ID, PartID: "part_running_recovery", Step: 1, Name: "bash", Status: store.ToolUseStatusProposed}
	for _, status := range []string{store.ToolUseStatusProposed, store.ToolUseStatusAuthorized, store.ToolUseStatusStarted} {
		use.Status = status
		if err := data.SaveToolUse(ctx, use); err != nil {
			t.Fatal(err)
		}
	}

	server := New(Config{Store: data})
	server.runs.reconcileInterval = time.Hour
	t.Cleanup(func() {
		server.shutdownCancel()
		server.runs.stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.runs.wait(waitCtx)
	})
	if err := server.recoverStartup(ctx); err != nil {
		t.Fatal(err)
	}

	recovered, err := data.GetSessionRun(ctx, admission.Run.SessionID, admission.Run.ID)
	if err != nil || recovered.Status != store.SessionRunStatusAborted || recovered.ErrorType != "process_interrupted" {
		t.Fatalf("run = %#v, error = %v", recovered, err)
	}
	calls, err := data.ListModelCalls(ctx, admission.Run.SessionID)
	if err != nil || len(calls) != 1 || calls[0].Status != store.ModelCallStatusAborted || calls[0].ErrorType != "process_interrupted" {
		t.Fatalf("model calls = %#v, error = %v", calls, err)
	}
	uses, err := data.ListToolUses(ctx, admission.Run.SessionID)
	if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
	}
	messages, err := data.ListMessages(ctx, admission.Run.SessionID)
	if err != nil || len(messages) != 1 || messages[0].State != "failed" || messages[0].Revision != 2 {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	events, err := data.ListSessionEvents(ctx, admission.Run.SessionID, 0, 10)
	if err != nil || len(events) != 3 || events[2].Type != "session.run.aborted" {
		t.Fatalf("events = %#v, error = %v", events, err)
	}
	if err := server.recoverStartup(ctx); err != nil {
		t.Fatal(err)
	}
	eventsAfterRetry, err := data.ListSessionEvents(ctx, admission.Run.SessionID, 0, 10)
	if err != nil || len(eventsAfterRetry) != len(events) {
		t.Fatalf("events after retry = %#v, error = %v", eventsAfterRetry, err)
	}
}

func TestSessionRunEndpointsAuthorizeAndAbortQueuedRun(t *testing.T) {
	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateClient("Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_runs_http", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_runs_http", Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})

	for _, test := range []struct {
		method, path, client string
		want                 int
	}{
		{http.MethodGet, "/sessions/ses_runs_http/runs", other.ID, http.StatusForbidden},
		{http.MethodGet, "/sessions/ses_runs_http/runs/missing", owner.ID, http.StatusNotFound},
		{http.MethodPost, "/sessions/ses_runs_http/runs/" + admission.Run.ID + "/abort", owner.ID, http.StatusOK},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("X-Wingman-Client", test.client)
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("%s %s status = %d, want %d: %s", test.method, test.path, response.Code, test.want, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/sessions/ses_runs_http/runs", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if strings.Contains(response.Body.String(), "request_hash") {
		t.Fatalf("request hash exposed: %s", response.Body.String())
	}
	var runs []store.SessionRun
	if err := json.NewDecoder(response.Body).Decode(&runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != store.SessionRunStatusAborted {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestSessionRunWorkerPublishesOnlyCommittedTransitions(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_run_events"}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_run_events", Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	server.runs.wake("ses_run_events")
	var events []store.SessionEvent
	deadline := time.After(time.Second)
	for {
		events, err = data.ListSessionEvents(context.Background(), "ses_run_events", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for worker events: %#v", events)
		case <-time.After(time.Millisecond):
		}
	}
	if len(events) != 3 || events[0].Type != "session.run.queued" || events[1].Type != "session.run.started" || events[2].Type != "session.run.failed" {
		t.Fatalf("events = %#v", events)
	}
	run, err := data.GetSessionRun(context.Background(), "ses_run_events", admission.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != store.SessionRunStatusFailed || run.ErrorType != "run_failed" || run.ErrorMessage == "" {
		t.Fatalf("run = %#v", run)
	}
}
