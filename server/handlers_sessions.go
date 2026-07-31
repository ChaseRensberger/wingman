package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/execution"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

const defaultSessionTitle = "New session"

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req api.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := req.Title
	if title == "" {
		title = defaultSessionTitle
	}

	workDir, workspaceID, err := s.resolveSessionLocation(req.WorkingDirectory, req.WorkspaceID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &store.Session{
		Title:       title,
		WorkDir:     workDir,
		WorkspaceID: workspaceID,
	}

	clientID, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess.ClientID = clientID

	if err := s.store.CreateSession(sess); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiSession(sess))
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var sessions []*store.Session
	var err error

	clientID, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sessions, err = s.store.ListSessionsByClient(clientID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiSessions(sessions))
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	history, err := s.sessionHistory(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	latestCall, err := s.store.LatestModelCall(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiSessionDetail(sess, history, latestCall))
}

func (s *Server) handleListSessionModelCalls(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}

	calls, err := s.store.ListModelCalls(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiModelCalls(calls))
}

func (s *Server) handleListSessionToolUses(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}

	uses, err := s.store.ListToolUses(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiToolUses(uses))
}

func (s *Server) sessionHistory(ctx context.Context, sessionID string) ([]models.Message, error) {
	storedMsgs, err := s.store.ListMessages(ctx, sessionID)
	if err != nil {
		if err == store.ErrSessionNotFound {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("list messages: %w", err)
	}
	calls, err := s.store.ListModelCalls(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list model calls: %w", err)
	}
	callsByMessageID := make(map[string]store.ModelCall, len(calls))
	for _, call := range calls {
		if call.AssistantMessageID != "" {
			callsByMessageID[call.AssistantMessageID] = call
		}
	}

	history := make([]models.Message, len(storedMsgs))
	for i, sm := range storedMsgs {
		msg, err := session.StoredMessageToModel(sm)
		if err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		if call, ok := callsByMessageID[sm.ID]; ok {
			session.ApplyModelCall(&msg, call)
		}
		if msg.Metadata == nil {
			msg.Metadata = models.Meta{}
		}
		msg.Metadata["message_id"] = sm.ID
		history[i] = msg
	}
	if history == nil {
		history = []models.Message{}
	}
	return models.NormalizeMessages(history), nil
}

func (s *Server) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}

	var req api.RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		s.writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.ExpectedVersion <= 0 {
		s.writeError(w, http.StatusBadRequest, "expected_version must be positive")
		return
	}
	sess, err := s.store.RenameSession(r.Context(), id, req.Title, req.ExpectedVersion)
	if s.writeSessionCommandError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, apiSession(sess))
}

func (s *Server) handleMoveSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	var req api.MoveSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.WorkingDirectory == nil) == (req.WorkspaceID == nil) {
		s.writeError(w, http.StatusBadRequest, "exactly one of working_directory or workspace_id is required")
		return
	}
	if req.ExpectedVersion <= 0 {
		s.writeError(w, http.StatusBadRequest, "expected_version must be positive")
		return
	}
	workingDirectory, workspaceID := "", ""
	if req.WorkingDirectory != nil {
		workingDirectory = *req.WorkingDirectory
	}
	if req.WorkspaceID != nil {
		workspaceID = *req.WorkspaceID
	}
	workDir, resolvedWorkspaceID, err := s.resolveSessionLocation(workingDirectory, workspaceID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.store.MoveSession(r.Context(), id, workDir, resolvedWorkspaceID, req.ExpectedVersion)
	if s.writeSessionCommandError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, apiSession(sess))
}

func (s *Server) writeSessionCommandError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrAggregateVersionConflict) {
		s.writeError(w, http.StatusConflict, err.Error())
		return true
	}
	if errors.Is(err, store.ErrSessionNotFound) {
		s.writeError(w, http.StatusNotFound, err.Error())
		return true
	}
	s.writeError(w, http.StatusInternalServerError, err.Error())
	return true
}

func (s *Server) resolveSessionLocation(workingDirectory, workspaceID string) (workDir string, resolvedWorkspaceID string, err error) {
	if workspaceID != "" {
		if workingDirectory != "" {
			return "", "", fmt.Errorf("working_directory and workspace_id cannot both be set")
		}
		workspace, err := s.store.GetWorkspace(workspaceID)
		if err != nil {
			return "", "", err
		}
		return workspace.Path, workspace.ID, nil
	}
	workDir, err = session.ResolveWorkDir(workingDirectory)
	if err != nil {
		return "", "", err
	}
	return workDir, "", nil
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	expectedVersion, err := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
	if err != nil || expectedVersion <= 0 {
		s.writeError(w, http.StatusBadRequest, "expected_version must be a positive integer")
		return
	}
	if err := s.store.PurgeSession(r.Context(), id, expectedVersion); err != nil {
		s.writeSessionCommandError(w, err)
		return
	}
	s.events.closeSession(id)
	if err := s.runs.stopAndWait(r.Context(), id); err != nil {
		s.logger.Warn("wait for purged session worker", "session_id", id, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, api.StatusResponse{Status: "deleted"})
}

func (s *Server) handleMessageSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.writeError(w, http.StatusNotImplemented, "persistence is disabled; use POST /run for ephemeral runs")
		return
	}
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sess.ClientID != clientID {
		s.writeError(w, http.StatusForbidden, "session belongs to another client")
		return
	}

	var req api.MessageSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		s.writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.RequestID != "" {
		if strings.TrimSpace(req.RequestID) == "" {
			s.writeError(w, http.StatusBadRequest, "request_id cannot be blank")
			return
		}
		if len(req.RequestID) > 200 {
			s.writeError(w, http.StatusBadRequest, "request_id must be 200 bytes or fewer")
			return
		}
	}
	if req.AgentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	storedAgent, err := s.store.GetAgent(req.AgentID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
		return
	}

	effectiveAgent := s.agentWithRequestModel(storedAgent, req.ModelRef, req.ModelRoute)
	validationSession, err := s.buildSession(r.Context(), effectiveAgent, sess)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	validationCtx, validationCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer validationCancel()
	if err := validationSession.Close(validationCtx); err != nil {
		s.logger.Error("close admission validation session", "session_id", sess.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "close admission validation session")
		return
	}
	var outputSchemaJSON []byte
	if req.OutputSchema != nil {
		outputSchemaJSON, err = json.Marshal(req.OutputSchema)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid output schema")
			return
		}
	}
	admission, err := s.store.AdmitSessionRun(r.Context(), store.SessionRun{
		SessionID:        id,
		RequestID:        req.RequestID,
		Message:          req.Message,
		Agent:            *effectiveAgent,
		OutputSchemaJSON: outputSchemaJSON,
	})
	if err != nil {
		if errors.Is(err, store.ErrSessionRunAdmissionConflict) {
			s.writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, store.ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if admission.Created {
		s.events.publish(admission.QueuedEvent)
	}
	if admission.Run.Status == store.SessionRunStatusQueued {
		s.runs.wake(id)
	}
	writeJSON(w, http.StatusAccepted, api.MessageSessionResponse{RunID: admission.Run.ID, Status: admission.Run.Status, SessionVersion: admission.SessionVersion})
}

func (s *Server) handleAbortSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	n := s.runs.abort(id)
	writeJSON(w, http.StatusOK, api.AbortSessionResponse{SessionID: id, Aborted: n})
}

func (s *Server) handleListSessionRuns(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	runs, err := s.store.ListSessionRuns(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiSessionRuns(runs))
}

func (s *Server) handleGetSessionRun(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id, runID := chi.URLParam(r, "id"), chi.URLParam(r, "runID")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	run, err := s.store.GetSessionRun(r.Context(), id, runID)
	if errors.Is(err, store.ErrSessionRunNotFound) {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiSessionRun(*run))
}

func (s *Server) handleAbortSessionRun(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id, runID := chi.URLParam(r, "id"), chi.URLParam(r, "runID")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	run, err := s.store.GetSessionRun(r.Context(), id, runID)
	if errors.Is(err, store.ErrSessionRunNotFound) {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch run.Status {
	case store.SessionRunStatusQueued:
		transition, err := s.store.SettleSessionRun(r.Context(), store.SessionRunSettlement{ID: run.ID, ExpectedStatus: store.SessionRunStatusQueued, Status: store.SessionRunStatusAborted, ErrorType: "cancelled", ErrorMessage: "run cancelled", EventData: map[string]any{"error_type": "cancelled", "error_message": "run cancelled"}})
		if errors.Is(err, store.ErrSessionRunTransitionConflict) {
			s.writeError(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if transition.Changed {
			s.events.publish(transition.Event)
		}
		writeJSON(w, http.StatusOK, apiSessionRun(transition.Run))
	case store.SessionRunStatusRunning:
		if s.runs.abort(id) == 0 {
			s.writeError(w, http.StatusConflict, "run is not active on this server")
			return
		}
		writeJSON(w, http.StatusAccepted, apiSessionRun(*run))
	default:
		s.writeError(w, http.StatusConflict, "run is already terminal")
	}
}

// handleRun is POST /run. It constructs an in-memory session from an
// inline agent spec (ephemeral mode) or an existing agent_id (normal
// mode), runs one turn, and streams events back via SSE. No session is
// persisted.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req api.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		s.writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	var storedAgent *store.Agent
	if req.AgentID != "" {
		if s.Ephemeral() {
			s.writeError(w, http.StatusBadRequest, "agent_id is not supported in ephemeral mode; provide an inline agent spec")
			return
		}
		a, err := s.store.GetAgent(req.AgentID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
			return
		}
		storedAgent = a
	} else if req.Agent != nil {
		storedAgent = storeAgent(req.Agent)
	} else {
		s.writeError(w, http.StatusBadRequest, "agent or agent_id is required")
		return
	}

	storedAgent = s.agentWithRequestModel(storedAgent, req.ModelRef, req.ModelRoute)
	if storedAgent.ModelRef == "" {
		s.writeError(w, http.StatusBadRequest, "model_ref is required when agent has no model_ref")
		return
	}

	workDir, err := session.ResolveWorkDir(req.WorkingDirectory)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &store.Session{
		ID:      store.NewID("eph_"),
		Title:   "ephemeral",
		WorkDir: workDir,
	}

	runSession, err := s.buildEphemeralSession(r.Context(), storedAgent, sess)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := runSession.Close(cleanupCtx); err != nil {
			s.logger.Error("close ephemeral run session", "session_id", sess.ID, "error", err)
		}
	}()

	if req.OutputSchema != nil {
		runSession.SetOutputSchema(&models.OutputSchema{
			Name:   req.OutputSchema.Name,
			Schema: req.OutputSchema.Schema,
		})
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	done := s.trackInflight()
	defer done()
	go func() {
		select {
		case <-s.ShutdownCtx().Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	stream, err := runSession.RunStream(ctx, req.Message)
	if err != nil {
		writeRunStreamError(w, flusher, err)
		return
	}

	for stream.Next() {
		event, err := canonicalRunStreamEvent(stream.Event(), w.Header().Get("X-Request-ID"))
		if err != nil {
			continue
		}
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
		flusher.Flush()
		if event.Type == api.RunStreamEventError {
			return
		}
	}

	if err := stream.Err(); err != nil {
		writeRunStreamError(w, flusher, err)
		return
	}

	result := stream.Result()
	doneEnvelope := api.RunStreamEvent{
		Type:    api.RunStreamEventDone,
		Version: session.EnvelopeVersion,
		Data:    api.RunDoneEventData{Usage: result.Usage, Steps: result.Steps},
	}
	doneData, _ := json.Marshal(doneEnvelope)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
	flusher.Flush()
}

func canonicalRunStreamEvent(event session.StreamEvent, requestID string) (api.RunStreamEvent, error) {
	result := api.RunStreamEvent{Type: api.RunStreamEventType(event.Type), Version: event.Version}
	switch data := event.Data.(type) {
	case run.IterationStartEvent:
		result.Data = api.RunIterationStartEventData{Step: data.Step}
	case run.IterationEndEvent:
		result.Data = api.RunIterationEndEventData{Step: data.Step, Turn: apiRunTurn(data.Turn)}
	case run.MessageEvent:
		result.Data = api.RunMessageEventData{Step: data.Step, Message: data.Message}
	case run.ToolUseProposedEvent:
		result.Data = api.RunToolProposedEventData{Call: apiRunToolCall(data.Call)}
	case run.ToolUseAuthorizedEvent:
		result.Data = api.RunToolAuthorizedEventData{Call: apiRunToolCall(data.Call)}
	case run.ToolExecutionStartEvent:
		result.Data = api.RunToolStartEventData{Call: apiRunToolCall(data.Call)}
	case run.ToolExecutionProgressEvent:
		result.Data = api.RunToolProgressEventData{CallID: data.CallID, ToolUseID: data.ToolUseID, Name: data.Name, OutputDelta: data.OutputDelta, Metadata: data.Metadata}
	case run.ToolExecutionEndEvent:
		result.Data = api.RunToolEndEventData{Result: apiRunToolResult(data.Result)}
	case run.StreamPartEvent:
		part, err := models.MarshalStreamPart(data.Part)
		if err != nil {
			return api.RunStreamEvent{}, err
		}
		result.Data = api.RunStreamPartEventData{Step: data.Step, MessageID: data.MessageID, PartID: data.PartID, Revision: data.Revision, Part: part}
	case run.ContextTransformedEvent:
		result.Data = api.RunContextTransformedEventData{Step: data.Step, Phase: data.Phase, OriginalCount: data.OriginalCount, NewCount: data.NewCount, Head: data.Head}
	case map[string]string:
		message := data["error"]
		if message == "" {
			message = "run failed"
		}
		result.Data = api.RunErrorEventData{Code: api.ErrorCodeRunFailed, Message: message, RequestID: requestID}
	case map[string]any:
		if event.Type == string(api.RunStreamEventStructuredOutput) {
			result.Data = api.RunStructuredOutputEventData{
				Schema: stringValue(data["schema"]), RawJSON: stringValue(data["raw_json"]), Parsed: mapValue(data["parsed"]),
			}
		} else {
			result.Data = api.UnknownRunStreamEventData{Value: data}
		}
	default:
		result.Data = api.UnknownRunStreamEventData{Value: data}
	}
	return result, nil
}

func apiRunTurn(value run.Turn) api.RunTurn {
	results := make([]api.RunToolResult, len(value.Results))
	for i, result := range value.Results {
		results[i] = apiRunToolResult(result)
	}
	errorMessage := ""
	if value.Failure != nil {
		errorMessage = value.Failure.Error()
	}
	return api.RunTurn{
		Step: value.Step, ModelCallID: value.ModelCallID, Attempt: value.Attempt,
		ProviderRequestID: value.ProviderRequestID, Assistant: value.Assistant, Results: results,
		Usage: value.Usage, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt,
		Trace: value.Trace, Error: errorMessage,
	}
}

func apiRunToolCall(value run.ToolCall) api.RunToolCall {
	return api.RunToolCall{
		CallID: value.ID, ToolUseID: value.ToolUseID, MessageID: value.MessageID, PartID: value.PartID,
		ModelCallID: value.ModelCallID, Step: value.Step, Ordinal: value.Ordinal,
		ProposedAt: formatEventTime(value.ProposedAt), AuthorizedAt: formatEventTime(value.AuthorizedAt), StartedAt: formatEventTime(value.StartedAt),
		Name: value.Name, Args: value.Args,
	}
}

func apiRunToolResult(value run.ToolResult) api.RunToolResult {
	return api.RunToolResult{
		CallID: value.CallID, ToolUseID: value.ToolUseID, Status: string(value.Status), Name: value.Name,
		Args: value.Args, Output: value.Output, Structured: value.Structured, Error: value.Error,
		ErrorType: value.ErrorType, Metadata: value.Metadata, IsError: value.IsError, Duration: int64(value.Duration),
	}
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func writeRunStreamError(w http.ResponseWriter, flusher http.Flusher, err error) {
	event := api.RunStreamEvent{
		Type:    api.RunStreamEventError,
		Version: session.EnvelopeVersion,
		Data: api.RunErrorEventData{
			Code: api.ErrorCodeRunFailed, Message: err.Error(), RequestID: w.Header().Get("X-Request-ID"),
		},
	}
	data, marshalErr := json.Marshal(event)
	if marshalErr == nil {
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
	}
}

// buildSession assembles a session.Session from a stored agent and the
// stored session record. It instantiates the model via the providers
// registry, resolves the tool registry, and wires persistence directly
// via WithStore so the session loads its history from disk on Run and
// persists every new message back as it lands.
func (s *Server) buildSession(ctx context.Context, stored *store.Agent, sess *store.Session) (*session.Session, error) {
	return s.buildSessionWithStore(ctx, stored, sess, s.store, "", s.permissionRequests.prompter(sess.ID, ""))
}

func (s *Server) buildSessionForRun(ctx context.Context, stored *store.Agent, sess *store.Session, runID string) (*session.Session, error) {
	return s.buildSessionWithStore(ctx, stored, sess, s.store, runID, s.permissionRequests.prompter(sess.ID, runID))
}

func (s *Server) buildEphemeralSession(ctx context.Context, stored *store.Agent, sess *store.Session) (*session.Session, error) {
	return s.buildSessionWithStore(ctx, stored, sess, nil, "", nil)
}

func (s *Server) buildSessionWithStore(ctx context.Context, stored *store.Agent, sess *store.Session, st store.Store, runID string, prompter run.PermissionPrompter) (*session.Session, error) {
	if stored.ModelRef == "" {
		return nil, fmt.Errorf("model_ref is required when agent has no model_ref")
	}
	executionScope, releaseScope, err := s.executionScope(ctx, sess.WorkDir)
	if err != nil {
		return nil, err
	}
	keepScope := false
	defer func() {
		if !keepScope {
			releaseScope()
		}
	}()
	providers := s.providers
	workDir := sess.WorkDir
	if executionScope != nil {
		providers = executionScope.Providers()
		workDir = executionScope.WorkDir()
	}

	modelRef, modelInfo, client, err := s.buildModelClient(stored, providers)
	if err != nil {
		return nil, err
	}

	opts := []session.Option{
		session.WithID(sess.ID),
		session.WithClient(client),
		session.WithModelRef(modelRef, modelInfo),
		session.WithSystem(stored.Instructions),
		session.WithWorkDir(workDir),
		session.WithPermissions(s.effectivePermissions(stored)),
		session.WithPermissionPrompter(prompter),
		session.WithLogger(s.logger.With("agent_id", stored.ID)),
		session.WithAgentID(stored.ID),
	}
	if st != nil {
		opts = append(opts, session.WithStore(st))
	}
	if runID != "" {
		opts = append(opts, session.WithRunID(runID))
	}
	tools, err := s.resolveTools(executionScope, stored.Tools)
	if err != nil {
		return nil, err
	}
	if len(tools) > 0 {
		opts = append(opts, session.WithTools(tools...))
	}
	if len(stored.OutputSchema) > 0 {
		opts = append(opts, session.WithOutputSchema(&models.OutputSchema{
			Name:   stored.ID,
			Schema: stored.OutputSchema,
			Strict: true,
		}))
	}
	opts = append(opts, session.WithCleanup(func(context.Context) error {
		releaseScope()
		return nil
	}))

	keepScope = true
	return session.New(opts...), nil
}

func (s *Server) effectivePermissions(agent *store.Agent) permission.Ruleset {
	if agent == nil {
		return s.permissions
	}
	return permission.Merge(
		agent.Permissions,
		s.permissions,
		s.agentPermissions[agent.Name],
		s.agentPermissions[agent.ID],
	)
}

// buildModelClient resolves a model ref and returns a route-backed model client.
func (s *Server) buildModelClient(stored *store.Agent, providers *provider.Registry) (models.ModelRef, models.ModelInfo, models.Client, error) {
	ref, ok := models.ParseModelRef(stored.ModelRef)
	if !ok {
		return models.ModelRef{}, models.ModelInfo{}, nil, fmt.Errorf("invalid model_ref: %s", stored.ModelRef)
	}
	info, err := s.resolveModelInfo(providers.Catalog(), ref, stored.Options)
	if err != nil {
		return models.ModelRef{}, models.ModelInfo{}, nil, err
	}
	ref = modelRefWithInfo(ref, info)
	var auth *store.Auth
	if s.store != nil {
		var err error
		auth, err = s.store.GetAuth()
		if err != nil {
			return models.ModelRef{}, models.ModelInfo{}, nil, fmt.Errorf("failed to load auth: %w", err)
		}
	} else {
		auth = &store.Auth{Providers: make(map[string]store.AuthCredential)}
	}
	credentials := map[string]provider.Credential{}
	for id, cred := range auth.Providers {
		credentials[id] = provider.Credential{
			Type: cred.Type, Key: cred.Key, Access: cred.Access, Refresh: cred.Refresh,
			ExpiresAt: cred.ExpiresAt, AccountID: cred.AccountID,
		}
	}
	return ref, info, providers.NewClientWithCredentials(credentials, s.refreshProviderCredential), nil
}

func (s *Server) resolveModelInfo(modelCatalog *catalog.Catalog, ref models.ModelRef, options map[string]any) (models.ModelInfo, error) {
	if info, ok := modelCatalog.Get(ref.Provider, ref.ID); ok {
		return info, nil
	}
	info, ok, err := modelRouteFromOptions(options)
	if err != nil {
		return models.ModelInfo{}, err
	}
	if !ok {
		return models.ModelInfo{}, fmt.Errorf("unknown model: %s; provide model_route.api and model_route.base_url for custom models", ref.Ref())
	}
	if info.Provider == "" {
		info.Provider = ref.Provider
	}
	if info.ID == "" {
		info.ID = ref.ID
	}
	if info.Provider != ref.Provider || info.ID != ref.ID {
		return models.ModelInfo{}, fmt.Errorf("model_route %s/%s does not match model_ref %s", info.Provider, info.ID, ref.Ref())
	}
	if info.API == "" || info.BaseURL == "" {
		return models.ModelInfo{}, fmt.Errorf("model_route for %s requires api and base_url", ref.Ref())
	}
	return info, nil
}

func modelRouteFromOptions(options map[string]any) (models.ModelInfo, bool, error) {
	raw, ok := options[agentOptionModelRoute]
	if !ok || raw == nil {
		return models.ModelInfo{}, false, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return models.ModelInfo{}, false, fmt.Errorf("invalid model_route: %w", err)
	}
	var info models.ModelInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return models.ModelInfo{}, false, fmt.Errorf("invalid model_route: %w", err)
	}
	return info, true, nil
}

func modelRefWithInfo(ref models.ModelRef, info models.ModelInfo) models.ModelRef {
	ref.API = info.API
	ref.BaseURL = info.BaseURL
	ref.Env = info.Env
	ref.ContextWindow = info.ContextWindow
	ref.MaxOutput = info.MaxOutput
	ref.Capabilities = info.Capabilities
	return ref
}

func (s *Server) agentWithRequestModel(stored *store.Agent, modelRef string, route *models.ModelInfo) *store.Agent {
	if modelRef == "" && route == nil {
		return stored
	}
	cp := *stored
	if modelRef != "" {
		cp.ModelRef = modelRef
	}
	if stored.Options != nil {
		cp.Options = map[string]any{}
		for k, v := range stored.Options {
			cp.Options[k] = v
		}
	}
	setAgentModelRoute(&cp, route)
	return &cp
}

// resolveTools maps stored names to one validated live catalog. A configured
// tool becoming unavailable is an explicit session construction error.
func (s *Server) resolveTools(scope *execution.Scope, toolNames []string) ([]tool.Tool, error) {
	var registry *tool.Registry
	var err error
	if scope != nil {
		registry, err = scope.ToolCatalog()
	} else {
		registry, _, err = s.toolCatalog(nil)
	}
	if err != nil {
		return nil, err
	}
	tools := make([]tool.Tool, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("agent tool %q is configured more than once", name)
		}
		seen[name] = struct{}{}
		t, err := registry.Get(name)
		if err != nil {
			return nil, fmt.Errorf("agent tool %q is unavailable: %w", name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}
