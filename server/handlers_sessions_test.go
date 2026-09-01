package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/chaserensberger/wingman/tool"
)

type sessionActionTestPlugin struct{}

func (sessionActionTestPlugin) Name() string { return "test" }

func (sessionActionTestPlugin) Activate(registry *plugin.Registry) (plugin.Cleanup, error) {
	return nil, registry.RegisterAction(plugin.Action{
		ID: "test.run", Command: "run", Description: "Run a test action",
		Handler: func(context.Context, plugin.ActionInfo) error { return nil },
	})
}

func TestSessionSummaryAndDetailUsePublicDTOs(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	stored := &store.Session{ID: "ses_public_dto", Title: "Contract", ClientID: client.ID}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	s := New(Config{Store: data})

	listResponse := httptest.NewRecorder()
	s.router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/sessions", nil))
	listBody := append([]byte(nil), listResponse.Body.Bytes()...)
	var summaries []api.Session
	if err := json.Unmarshal(listBody, &summaries); err != nil {
		t.Fatal(err)
	}
	if listResponse.Code != http.StatusOK || len(summaries) != 1 || summaries[0].Version != 1 {
		t.Fatalf("status = %d, sessions = %#v", listResponse.Code, summaries)
	}
	if strings.Contains(string(listBody), `"history"`) {
		t.Fatalf("summary response contains history: %s", listBody)
	}

	detailResponse := httptest.NewRecorder()
	s.router.ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/sessions/ses_public_dto", nil))
	var detail api.SessionDetail
	if err := json.NewDecoder(detailResponse.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detailResponse.Code != http.StatusOK || detail.ID != stored.ID || detail.History == nil || len(detail.History) != 0 {
		t.Fatalf("status = %d, detail = %#v", detailResponse.Code, detail)
	}
}

func TestCanonicalRunStreamEventUsesPublicPayloadShape(t *testing.T) {
	event, err := canonicalRunStreamEvent(session.StreamEvent{
		Type: "context_transformed", Version: session.EnvelopeVersion,
		Data: run.ContextTransformedEvent{Step: 2, Phase: "before_step", OriginalCount: 4, NewCount: 2},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"Step"`) || !strings.Contains(string(encoded), `"original_count":4`) {
		t.Fatalf("event = %s", encoded)
	}

	event, err = canonicalRunStreamEvent(session.StreamEvent{
		Type: "stream_part", Version: session.EnvelopeVersion,
		Data: run.StreamPartEvent{Step: 1, MessageID: "msg_1", Part: models.TextDeltaPart{Delta: "hi"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"type":"text_delta"`) || !strings.Contains(string(encoded), `"delta":"hi"`) {
		t.Fatalf("event = %s", encoded)
	}
}

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
	case <-live.done:
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
	var first api.MessageSessionResponse
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
	var second api.MessageSessionResponse
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

func TestSessionActionCatalogAndAdmissionAreGenericAndIdempotent(t *testing.T) {
	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_action_http", ClientID: client.ID}); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateAgent(&store.Agent{
		ID: "agt_action_http", Name: "Action", ModelRef: "test/model",
		Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
			Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Store: &admissionTestStore{Store: data},
		PluginFactories: []func() plugin.Plugin{
			func() plugin.Plugin { return sessionActionTestPlugin{} },
		},
	})

	catalogResponse := httptest.NewRecorder()
	server.router.ServeHTTP(catalogResponse, httptest.NewRequest(http.MethodGet, "/actions", nil))
	var catalog api.ActionsResponse
	if err := json.NewDecoder(catalogResponse.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if catalogResponse.Code != http.StatusOK || len(catalog.Actions) != 1 || catalog.Actions[0].ID != "test.run" || catalog.Actions[0].Command != "run" {
		t.Fatalf("status = %d, catalog = %#v", catalogResponse.Code, catalog)
	}

	admit := func(body string) (int, api.ActionSessionResponse) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/sessions/ses_action_http/actions/test.run", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		var result api.ActionSessionResponse
		if response.Code == http.StatusAccepted {
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return response.Code, result
	}
	status, first := admit(`{"request_id":"action-request","agent_id":"agt_action_http","input":{"value":1}}`)
	if status != http.StatusAccepted || first.RunID == "" {
		t.Fatalf("status = %d, admission = %#v", status, first)
	}
	status, retry := admit(`{"request_id":"action-request","agent_id":"agt_action_http","input": { "value": 1 }}`)
	if status != http.StatusAccepted || retry.RunID != first.RunID {
		t.Fatalf("retry status = %d, retry = %#v, first = %#v", status, retry, first)
	}
	status, _ = admit(`{"request_id":"action-request","agent_id":"agt_action_http","input":{"value":2}}`)
	if status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d", status, http.StatusConflict)
	}
	stored, err := data.GetSessionRun(context.Background(), "ses_action_http", first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Kind != store.SessionRunKindAction || stored.Action != "test.run" || string(stored.InputJSON) != `{"value":1}` {
		t.Fatalf("stored run = %#v", stored)
	}
	aliasRequest := httptest.NewRequest(http.MethodPost, "/sessions/ses_action_http/actions/run", strings.NewReader(`{"request_id":"action-alias","agent_id":"agt_action_http"}`))
	aliasRequest.Header.Set("Content-Type", "application/json")
	aliasResponse := httptest.NewRecorder()
	server.router.ServeHTTP(aliasResponse, aliasRequest)
	var aliasAdmission api.ActionSessionResponse
	if err := json.NewDecoder(aliasResponse.Body).Decode(&aliasAdmission); err != nil {
		t.Fatal(err)
	}
	aliasRun, err := data.GetSessionRun(context.Background(), "ses_action_http", aliasAdmission.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if aliasResponse.Code != http.StatusAccepted || aliasRun.Action != "test.run" {
		t.Fatalf("alias status = %d, run = %#v", aliasResponse.Code, aliasRun)
	}
}

func TestMessageSessionSnapshotsProjectInstructions(t *testing.T) {
	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	projectPath := filepath.Join(workDir, agentsFileName)
	if err := os.WriteFile(projectPath, []byte("Initial project rules."), 0o644); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(workDir, ".wingman", "skills", "review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\ndescription: Review code.\n---\nInitial skill instructions."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_instruction_snapshot", ClientID: client.ID, WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateAgent(&store.Agent{
		ID: "agt_instruction_snapshot", Name: "Snapshot", Instructions: "Agent rules.", ModelRef: "test/model",
		Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
			Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: &admissionTestStore{Store: data}})
	body := `{"request_id":"instruction-retry","agent_id":"agt_instruction_snapshot","message":"hello"}`
	request := httptest.NewRequest(http.MethodPost, "/sessions/ses_instruction_snapshot/message", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	var admitted api.MessageSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&admitted); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("Changed project rules."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\ndescription: Review code.\n---\nChanged skill instructions."), 0o644); err != nil {
		t.Fatal(err)
	}
	retryResponse := httptest.NewRecorder()
	server.router.ServeHTTP(retryResponse, httptest.NewRequest(http.MethodPost, "/sessions/ses_instruction_snapshot/message", strings.NewReader(body)))
	var retry api.MessageSessionResponse
	if err := json.NewDecoder(retryResponse.Body).Decode(&retry); err != nil {
		t.Fatal(err)
	}
	if retryResponse.Code != http.StatusAccepted || retry.RunID != admitted.RunID {
		t.Fatalf("retry status = %d, retry = %#v", retryResponse.Code, retry)
	}
	if err := os.Remove(projectPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	unreadableRetry := httptest.NewRecorder()
	server.router.ServeHTTP(unreadableRetry, httptest.NewRequest(http.MethodPost, "/sessions/ses_instruction_snapshot/message", strings.NewReader(body)))
	if unreadableRetry.Code != http.StatusAccepted {
		t.Fatalf("unreadable retry status = %d: %s", unreadableRetry.Code, unreadableRetry.Body.String())
	}
	run, err := data.GetSessionRun(context.Background(), "ses_instruction_snapshot", admitted.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Agent.Instructions != "Agent rules." {
		t.Fatalf("authored agent instructions = %q", run.Agent.Instructions)
	}
	if !strings.Contains(run.EffectiveInstructions, "Initial project rules.") || strings.Contains(run.EffectiveInstructions, "Changed project rules.") {
		t.Fatalf("effective instructions = %q", run.EffectiveInstructions)
	}
	if len(run.Skills) != 1 || run.Skills[0].ID != "review" || run.Skills[0].Content != "Initial skill instructions." {
		t.Fatalf("skills = %#v", run.Skills)
	}
	canonicalProjectPath, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.InstructionSources) != 1 || run.InstructionSources[0].Path != canonicalProjectPath || run.InstructionSources[0].Kind != "project" {
		t.Fatalf("instruction sources = %#v", run.InstructionSources)
	}
	getResponse := httptest.NewRecorder()
	server.router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/sessions/ses_instruction_snapshot/runs/"+admitted.RunID, nil))
	var publicRun api.SessionRun
	if err := json.NewDecoder(getResponse.Body).Decode(&publicRun); err != nil {
		t.Fatal(err)
	}
	if getResponse.Code != http.StatusOK || !strings.Contains(publicRun.EffectiveInstructions, "Initial project rules.") || len(publicRun.InstructionSources) != 1 || publicRun.InstructionSources[0].SHA256 == "" {
		t.Fatalf("status = %d, run = %#v", getResponse.Code, publicRun)
	}
}

func TestSessionMacrosListAndAdmitExpandedMessage(t *testing.T) {
	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	macroPath := filepath.Join(workDir, ".wingman", "macros", "review.md")
	if err := os.MkdirAll(filepath.Dir(macroPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(macroPath, []byte("---\ndescription: Review a change\nagent: agt_macro\n---\nReview $ARGUMENTS. Focus on missing tests."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_macro", ClientID: client.ID, WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"agt_fallback", "agt_macro"} {
		if err := data.CreateAgent(&store.Agent{ID: id, Name: id, ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
			Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1",
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	server := New(Config{Store: &admissionTestStore{Store: data}})

	listResponse := httptest.NewRecorder()
	server.router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/sessions/ses_macro/macros", nil))
	var macros []api.Macro
	if err := json.NewDecoder(listResponse.Body).Decode(&macros); err != nil {
		t.Fatal(err)
	}
	if listResponse.Code != http.StatusOK || len(macros) != 1 || macros[0].ID != "review" || macros[0].AgentID != "agt_macro" || macros[0].Description != "Review a change" {
		t.Fatalf("status = %d, macros = %#v", listResponse.Code, macros)
	}

	body := `{"request_id":"macro-request","macro_id":"review","arguments":"the auth middleware","agent_id":"agt_fallback","model_ref":"test/model"}`
	request := httptest.NewRequest(http.MethodPost, "/sessions/ses_macro/macros", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var admitted api.MessageSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&admitted); err != nil {
		t.Fatal(err)
	}
	run, err := data.GetSessionRun(context.Background(), "ses_macro", admitted.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Message != "Review the auth middleware. Focus on missing tests." || run.Agent.ID != "agt_macro" {
		t.Fatalf("run = %#v", run)
	}

	retry := httptest.NewRecorder()
	server.router.ServeHTTP(retry, httptest.NewRequest(http.MethodPost, "/sessions/ses_macro/macros", strings.NewReader(body)))
	var repeated api.MessageSessionResponse
	if err := json.NewDecoder(retry.Body).Decode(&repeated); err != nil {
		t.Fatal(err)
	}
	if retry.Code != http.StatusAccepted || repeated.RunID != admitted.RunID {
		t.Fatalf("status = %d, response = %#v", retry.Code, repeated)
	}
}

func TestMessageSessionRejectsDirectoryScopedAgentWithoutWorkingDirectory(t *testing.T) {
	t.Parallel()

	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_no_workdir", ClientID: client.ID}); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateAgent(&store.Agent{
		ID:       "agt_needs_workdir",
		Name:     "Needs workdir",
		ModelRef: "test/model",
		Tools:    []string{"read"},
		Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
			Provider: "test",
			ID:       "model",
			API:      models.APIOpenAICompatible,
			BaseURL:  "http://127.0.0.1:1",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	request := httptest.NewRequest(http.MethodPost, "/sessions/ses_no_workdir/message", strings.NewReader(`{"agent_id":"agt_needs_workdir","message":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `tool \"read\" requires a working directory`) {
		t.Fatalf("response = %s", response.Body.String())
	}
	runs, err := data.ListSessionRuns(context.Background(), "ses_no_workdir")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %#v, want none", runs)
	}
	session, err := data.GetSession("ses_no_workdir")
	if err != nil {
		t.Fatal(err)
	}
	if session.AggregateVersion != 1 {
		t.Fatalf("session version = %d, want 1", session.AggregateVersion)
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
	var calls []api.ModelCall
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
	case event := <-events.events:
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

type startedToolClient struct{}

func (startedToolClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (startedToolClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (startedToolClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	message := &models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{ID: "part_started_tool_recovery", CallID: "call_started_tool_recovery", Name: "test", Input: map[string]any{}}}}
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(message, nil)
	return stream, nil
}

type crashDispatchClient struct{ marker string }

func (crashDispatchClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (crashDispatchClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c crashDispatchClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	if err := os.WriteFile(c.marker, []byte("1"), 0o600); err != nil {
		return nil, err
	}
	os.Exit(0)
	return nil, nil
}

type startupRecoveryStore struct {
	store.Store
	order         []string
	permissionErr error
	interruptErr  error
	listErr       error
	resumeErr     error
}

func (s *startupRecoveryStore) InterruptPendingPermissionRequests(context.Context) ([]store.PermissionRequestTransition, error) {
	s.order = append(s.order, "permissions")
	if s.permissionErr != nil {
		return nil, s.permissionErr
	}
	return s.Store.InterruptPendingPermissionRequests(context.Background())
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
	if _, err := data.CreatePermissionRequest(context.Background(), store.PermissionRequest{ID: "prq_recovery", SessionID: "ses_recovery", Action: "edit", Resources: []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	recoveryStore := &startupRecoveryStore{Store: data}
	server := New(Config{Store: recoveryStore})
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(recoveryStore.order, ","), "permissions,interrupt,list,resume"; got != want {
		t.Fatalf("recovery order = %q, want %q", got, want)
	}
	uses, err := data.ListToolUses(context.Background(), "ses_recovery")
	if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
	}
	requests, err := data.ListPermissionRequests(context.Background(), "ses_recovery")
	if err != nil || len(requests) != 1 || requests[0].Status != store.PermissionRequestStatusInterrupted {
		t.Fatalf("permission requests = %#v, error = %v", requests, err)
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
		name       string
		permission error
		interrupt  error
		list       error
		resume     error
		wantOrder  string
	}{
		{"permissions", errors.New("permissions failed"), nil, nil, nil, "permissions"},
		{"interrupt", nil, errors.New("interrupt failed"), nil, nil, "permissions,interrupt"},
		{"list", nil, nil, errors.New("list failed"), nil, "permissions,interrupt,list"},
		{"resume", nil, nil, nil, errors.New("resume failed"), "permissions,interrupt,list,resume"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recoveryStore := &startupRecoveryStore{Store: memory.NewStore(), permissionErr: test.permission, interruptErr: test.interrupt, listErr: test.list, resumeErr: test.resume}
			server := New(Config{Store: recoveryStore})
			err := server.recoverStartup(context.Background())
			if !errors.Is(err, test.permission) && !errors.Is(err, test.interrupt) && !errors.Is(err, test.list) && !errors.Is(err, test.resume) {
				t.Fatalf("error = %v, want wrapped recovery failure", err)
			}
			if got := strings.Join(recoveryStore.order, ","); got != test.wantOrder {
				t.Fatalf("recovery order = %q, want %q", got, test.wantOrder)
			}
		})
	}
}

func TestPermissionRequestEndpointsAuthorizeReplyAndPublishOnce(t *testing.T) {
	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateClient("permission other")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_permission_http", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_permission_http_empty", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	request, err := data.CreatePermissionRequest(context.Background(), store.PermissionRequest{SessionID: "ses_permission_http", Action: "edit", Resources: []string{"a.go"}})
	if err != nil {
		t.Fatal(err)
	}

	call := func(method, path, body, client string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Wingman-Client", client)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, r)
		return w
	}
	if response := call(http.MethodGet, "/sessions/ses_permission_http/permission-requests", "", other.ID); response.Code != http.StatusForbidden {
		t.Fatalf("other client status = %d", response.Code)
	}
	if response := call(http.MethodGet, "/sessions/ses_permission_http_empty/permission-requests", "", owner.ID); response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("empty list response = %d %s", response.Code, response.Body.String())
	}
	response := call(http.MethodGet, "/sessions/ses_permission_http/permission-requests", "", owner.ID)
	if response.Code != http.StatusOK || response.Body.String() == "null\n" {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}
	var requests []store.PermissionRequest
	if err := json.NewDecoder(response.Body).Decode(&requests); err != nil || len(requests) != 1 || requests[0].ID != request.Request.ID {
		t.Fatalf("requests = %#v, %v", requests, err)
	}
	if response := call(http.MethodGet, "/sessions/ses_permission_http/permission-grants", "", owner.ID); response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("grants response = %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/sessions/ses_permission_http/permission-requests/"+request.Request.ID+"/reply", `{"response":"bad"}`, owner.ID); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid reply status = %d", response.Code)
	}
	if response := call(http.MethodPost, "/sessions/ses_permission_http/permission-requests/missing/reply", `{"response":"once"}`, owner.ID); response.Code != http.StatusNotFound {
		t.Fatalf("missing reply status = %d", response.Code)
	}
	response = call(http.MethodPost, "/sessions/ses_permission_http/permission-requests/"+request.Request.ID+"/reply", `{"response":"always"}`, owner.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("reply status = %d: %s", response.Code, response.Body.String())
	}
	var resolved store.PermissionRequest
	if err := json.NewDecoder(response.Body).Decode(&resolved); err != nil || resolved.Status != store.PermissionRequestStatusApproved || resolved.Response != store.PermissionResponseAlways {
		t.Fatalf("resolved = %#v, %v", resolved, err)
	}
	if response := call(http.MethodPost, "/sessions/ses_permission_http/permission-requests/"+request.Request.ID+"/reply", `{"response":"always"}`, owner.ID); response.Code != http.StatusOK {
		t.Fatalf("identical retry status = %d", response.Code)
	}
	if response := call(http.MethodPost, "/sessions/ses_permission_http/permission-requests/"+request.Request.ID+"/reply", `{"response":"reject"}`, owner.ID); response.Code != http.StatusConflict {
		t.Fatalf("conflicting retry status = %d", response.Code)
	}
	grants := call(http.MethodGet, "/sessions/ses_permission_http/permission-grants", "", owner.ID)
	var listedGrants []store.PermissionGrant
	if err := json.NewDecoder(grants.Body).Decode(&listedGrants); err != nil || len(listedGrants) != 1 || listedGrants[0].Action != "edit" || listedGrants[0].Resource != "a.go" {
		t.Fatalf("grants = %#v, %v", listedGrants, err)
	}
	events, err := data.ListSessionEvents(context.Background(), "ses_permission_http", 0, 10)
	if err != nil || len(events) != 2 || events[0].Type != "session.permission.requested" || events[1].Type != "session.permission.resolved" {
		t.Fatalf("events = %#v, %v", events, err)
	}
	var eventRequest store.PermissionRequest
	if err := json.Unmarshal(events[0].DataJSON, &eventRequest); err != nil || eventRequest.ID != request.Request.ID || eventRequest.Action != "edit" || len(eventRequest.Resources) != 1 || eventRequest.Resources[0] != "a.go" {
		t.Fatalf("requested event = %#v, %v", eventRequest, err)
	}

	if err := data.CreateSession(&store.Session{ID: "ses_permission_retry_notify", ClientID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		response run.PermissionResponse
		err      error
	}, 1)
	go func() {
		response, err := server.permissionRequests.prompter("ses_permission_retry_notify", "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"b.go"}})
		result <- struct {
			response run.PermissionResponse
			err      error
		}{response, err}
	}()
	lostNotification := waitForPermissionRequest(t, data, "ses_permission_retry_notify")
	if _, err := data.ResolvePermissionRequest(context.Background(), store.PermissionRequestResolution{SessionID: lostNotification.SessionID, RequestID: lostNotification.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseOnce}); err != nil {
		t.Fatal(err)
	}
	if response := call(http.MethodPost, "/sessions/ses_permission_retry_notify/permission-requests/"+lostNotification.ID+"/reply", `{"response":"once"}`, owner.ID); response.Code != http.StatusOK {
		t.Fatalf("retry notification status = %d", response.Code)
	}
	got := <-result
	if got.err != nil || got.response != run.PermissionResponseOnce {
		t.Fatalf("retry notification result = %#v", got)
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

func TestStartupRecoveryAfterToolSideEffectDoesNotReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	marker := filepath.Join(t.TempDir(), "side-effect")
	command := exec.Command(os.Args[0], "-test.run=TestStartupRecoveryAfterToolSideEffectDoesNotReplayHelper")
	command.Env = append(os.Environ(), "GO_WANT_TOOL_SIDE_EFFECT_CRASH=1", "WINGMAN_TEST_DB="+path, "WINGMAN_TEST_MARKER="+marker)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash helper failed: %v\n%s", err, output)
	}
	if marker, err := os.ReadFile(marker); err != nil || string(marker) != "1" {
		t.Fatalf("side effect = %q, error = %v", marker, err)
	}

	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	server := New(Config{Store: data})
	server.runs.reconcileInterval = time.Hour
	t.Cleanup(func() {
		server.shutdownCancel()
		server.runs.stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.runs.wait(waitCtx)
	})
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}

	run, err := data.GetSessionRun(context.Background(), "ses_crash_side_effect", "run_crash_side_effect")
	if err != nil || run.Status != store.SessionRunStatusAborted || run.ErrorType != "process_interrupted" {
		t.Fatalf("run = %#v, error = %v", run, err)
	}
	uses, err := data.ListToolUses(context.Background(), "ses_crash_side_effect")
	if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted || uses[0].ErrorType != "process_interrupted" {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
	}
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if marker, err := os.ReadFile(marker); err != nil || string(marker) != "1" {
		t.Fatalf("side effect after recovery = %q, error = %v", marker, err)
	}
}

func TestStartupRecoveryAfterProviderDispatchDoesNotRedispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	marker := filepath.Join(t.TempDir(), "dispatch")
	command := exec.Command(os.Args[0], "-test.run=TestStartupRecoveryAfterProviderDispatchDoesNotRedispatchHelper")
	command.Env = append(os.Environ(), "GO_WANT_PROVIDER_DISPATCH_CRASH=1", "WINGMAN_TEST_DB="+path, "WINGMAN_TEST_MARKER="+marker)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash helper failed: %v\n%s", err, output)
	}
	if marker, err := os.ReadFile(marker); err != nil || string(marker) != "1" {
		t.Fatalf("provider dispatch = %q, error = %v", marker, err)
	}

	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	server := New(Config{Store: data})
	server.runs.reconcileInterval = time.Hour
	t.Cleanup(func() {
		server.shutdownCancel()
		server.runs.stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.runs.wait(waitCtx)
	})
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}

	run, err := data.GetSessionRun(context.Background(), "ses_crash_dispatch", "run_crash_dispatch")
	if err != nil || run.Status != store.SessionRunStatusAborted || run.ErrorType != "process_interrupted" {
		t.Fatalf("run = %#v, error = %v", run, err)
	}
	calls, err := data.ListModelCalls(context.Background(), "ses_crash_dispatch")
	if err != nil || len(calls) != 1 || calls[0].Status != store.ModelCallStatusAborted || calls[0].ErrorType != "process_interrupted" {
		t.Fatalf("model calls = %#v, error = %v", calls, err)
	}
	if err := server.recoverStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if marker, err := os.ReadFile(marker); err != nil || string(marker) != "1" {
		t.Fatalf("provider dispatch after recovery = %q, error = %v", marker, err)
	}
}

func TestStartupRecoveryAfterToolSideEffectDoesNotReplayHelper(t *testing.T) {
	if os.Getenv("GO_WANT_TOOL_SIDE_EFFECT_CRASH") != "1" {
		return
	}
	data, err := store.NewSQLiteStore(os.Getenv("WINGMAN_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_crash_side_effect"}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_crash_side_effect", SessionID: "ses_crash_side_effect", Message: "run tool"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(context.Background(), admission.Run.SessionID); err != nil {
		t.Fatal(err)
	}
	marker := os.Getenv("WINGMAN_TEST_MARKER")
	sess := session.New(
		session.WithID(admission.Run.SessionID),
		session.WithRunID(admission.Run.ID),
		session.WithStore(data),
		session.WithClient(startedToolClient{}),
		session.WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
		session.WithTools(tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
			if err := os.WriteFile(marker, []byte("1"), 0o600); err != nil {
				return tool.Result{}, err
			}
			os.Exit(0)
			return tool.Result{}, nil
		})),
	)
	_, err = sess.Run(context.Background(), admission.Run.Message)
	t.Fatalf("tool did not terminate the crash helper: %v", err)
}

func TestStartupRecoveryAfterProviderDispatchDoesNotRedispatchHelper(t *testing.T) {
	if os.Getenv("GO_WANT_PROVIDER_DISPATCH_CRASH") != "1" {
		return
	}
	data, err := store.NewSQLiteStore(os.Getenv("WINGMAN_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_crash_dispatch"}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_crash_dispatch", SessionID: "ses_crash_dispatch", Message: "dispatch"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(context.Background(), admission.Run.SessionID); err != nil {
		t.Fatal(err)
	}
	sess := session.New(
		session.WithID(admission.Run.SessionID),
		session.WithRunID(admission.Run.ID),
		session.WithStore(data),
		session.WithClient(crashDispatchClient{marker: os.Getenv("WINGMAN_TEST_MARKER")}),
		session.WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	_, err = sess.Run(context.Background(), admission.Run.Message)
	t.Fatalf("provider dispatch did not terminate the crash helper: %v", err)
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
