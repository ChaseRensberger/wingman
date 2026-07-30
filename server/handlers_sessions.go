package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"

	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/deepseek"
	_ "github.com/chaserensberger/wingman/models/providers/google"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/openaicompat"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
	_ "github.com/chaserensberger/wingman/models/providers/opencodego"
	_ "github.com/chaserensberger/wingman/models/providers/openrouter"
)

type CreateSessionRequest struct {
	Title            string `json:"title,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
}

const defaultSessionTitle = "New session"

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := req.Title
	if title == "" {
		title = defaultSessionTitle
	}

	workDir, workspaceID, err := s.resolveSessionLocation(req.WorkingDirectory, req.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &store.Session{
		Title:       title,
		WorkDir:     workDir,
		WorkspaceID: workspaceID,
	}

	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess.ClientID = clientID

	if err := s.store.CreateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sess)
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sessions, err = s.store.ListSessionsByClient(clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []*store.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	history, err := s.sessionHistory(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	latestCall, err := s.store.LatestModelCall(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SessionDetailResponse{
		Session:         sess,
		History:         history,
		LatestModelCall: latestCall,
	})
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, calls)
}

type SessionDetailResponse struct {
	*store.Session
	History         []models.Message `json:"history"`
	LatestModelCall *store.ModelCall `json:"latest_model_call,omitempty"`
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

type RenameSessionRequest struct {
	Title           string `json:"title"`
	ExpectedVersion int64  `json:"expected_version"`
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

	var req RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.ExpectedVersion <= 0 {
		writeError(w, http.StatusBadRequest, "expected_version must be positive")
		return
	}
	sess, err := s.store.RenameSession(r.Context(), id, req.Title, req.ExpectedVersion)
	if writeSessionCommandError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

type MoveSessionRequest struct {
	WorkingDirectory *string `json:"working_directory,omitempty"`
	WorkspaceID      *string `json:"workspace_id,omitempty"`
	ExpectedVersion  int64   `json:"expected_version"`
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
	var req MoveSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.WorkingDirectory == nil) == (req.WorkspaceID == nil) {
		writeError(w, http.StatusBadRequest, "exactly one of working_directory or workspace_id is required")
		return
	}
	if req.ExpectedVersion <= 0 {
		writeError(w, http.StatusBadRequest, "expected_version must be positive")
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.store.MoveSession(r.Context(), id, workDir, resolvedWorkspaceID, req.ExpectedVersion)
	if writeSessionCommandError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func writeSessionCommandError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrAggregateVersionConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return true
	}
	if errors.Is(err, store.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return true
	}
	writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusBadRequest, "expected_version must be a positive integer")
		return
	}
	if err := s.store.PurgeSession(r.Context(), id, expectedVersion); err != nil {
		writeSessionCommandError(w, err)
		return
	}
	s.events.closeSession(id)
	if err := s.runs.stopAndWait(r.Context(), id); err != nil {
		s.logger.Warn("wait for purged session worker", "session_id", id, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type messageOutputSchema struct {
	Name   string         `json:"name,omitempty"`
	Schema map[string]any `json:"schema"`
}

type MessageSessionRequest struct {
	AgentID      string               `json:"agent_id"`
	ModelRef     string               `json:"model_ref,omitempty"`
	ModelRoute   *models.ModelInfo    `json:"model_route,omitempty"`
	Message      string               `json:"message"`
	OutputSchema *messageOutputSchema `json:"output_schema,omitempty"`
}

type MessageSessionResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

// RunRequest is the body for POST /run. In ephemeral mode agent is
// required and agent_id is rejected. In normal mode either agent_id
// (looked up from the store) or agent (inline spec) is accepted.
type RunRequest struct {
	AgentID          string               `json:"agent_id,omitempty"`
	Agent            *store.Agent         `json:"agent,omitempty"`
	ModelRef         string               `json:"model_ref,omitempty"`
	ModelRoute       *models.ModelInfo    `json:"model_route,omitempty"`
	Message          string               `json:"message"`
	OutputSchema     *messageOutputSchema `json:"output_schema,omitempty"`
	WorkingDirectory string               `json:"working_directory,omitempty"`
}

func (s *Server) handleMessageSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		writeError(w, http.StatusNotImplemented, "persistence is disabled; use POST /run for ephemeral runs")
		return
	}
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sess.ClientID != clientID {
		writeError(w, http.StatusForbidden, "session belongs to another client")
		return
	}

	var req MessageSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	storedAgent, err := s.store.GetAgent(req.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
		return
	}

	effectiveAgent := s.agentWithRequestModel(storedAgent, req.ModelRef, req.ModelRoute)
	if _, err := s.buildSession(effectiveAgent, sess); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var outputSchemaJSON []byte
	if req.OutputSchema != nil {
		outputSchemaJSON, err = json.Marshal(req.OutputSchema)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid output schema")
			return
		}
	}
	queued, err := s.store.CreateSessionRun(r.Context(), store.SessionRun{
		SessionID:        id,
		Message:          req.Message,
		Agent:            *effectiveAgent,
		OutputSchemaJSON: outputSchemaJSON,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.persistRunEvent(context.Background(), id, "session.run.queued", map[string]any{"run_id": queued.ID, "message": queued.Message})
	s.runs.wake(id)
	writeJSON(w, http.StatusAccepted, MessageSessionResponse{RunID: queued.ID, Status: queued.Status})
}

// AbortSessionResponse reports how many in-flight runs were cancelled.
// Aborted is 0 when no run was active for the session; the request
// still returns 200 because cancellation is idempotent — clients
// shouldn't have to coordinate to issue an abort.
type AbortSessionResponse struct {
	SessionID string `json:"session_id"`
	Aborted   int    `json:"aborted"`
}

func (s *Server) handleAbortSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	// Verify the session exists so callers get a 404 for typos rather
	// than a misleading 200/aborted=0. Cheap lookup vs. silent miss.
	if _, err := s.store.GetSession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	n := s.runs.abort(id)
	writeJSON(w, http.StatusOK, AbortSessionResponse{SessionID: id, Aborted: n})
}

// handleRun is POST /run. It constructs an in-memory session from an
// inline agent spec (ephemeral mode) or an existing agent_id (normal
// mode), runs one turn, and streams events back via SSE. No session is
// persisted.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	var storedAgent *store.Agent
	if req.AgentID != "" {
		if s.Ephemeral() {
			writeError(w, http.StatusBadRequest, "agent_id is not supported in ephemeral mode; provide an inline agent spec")
			return
		}
		a, err := s.store.GetAgent(req.AgentID)
		if err != nil {
			writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
			return
		}
		storedAgent = a
	} else if req.Agent != nil {
		storedAgent = req.Agent
	} else {
		writeError(w, http.StatusBadRequest, "agent or agent_id is required")
		return
	}

	storedAgent = s.agentWithRequestModel(storedAgent, req.ModelRef, req.ModelRoute)
	if storedAgent.ModelRef == "" {
		writeError(w, http.StatusBadRequest, "model_ref is required when agent has no model_ref")
		return
	}

	workDir, err := session.ResolveWorkDir(req.WorkingDirectory)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &store.Session{
		ID:      store.NewID("eph_"),
		Title:   "ephemeral",
		WorkDir: workDir,
	}

	runSession, err := s.buildEphemeralSession(storedAgent, sess)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

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
		writeError(w, http.StatusInternalServerError, "streaming not supported")
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
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	for stream.Next() {
		event := stream.Event()
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
		flusher.Flush()
	}

	if err := stream.Err(); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	result := stream.Result()
	doneEnvelope := session.StreamEvent{
		Type:    "done",
		Version: session.EnvelopeVersion,
		Data: map[string]any{
			"usage": result.Usage,
			"steps": result.Steps,
		},
	}
	doneData, _ := json.Marshal(doneEnvelope)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
	flusher.Flush()
}

// buildSession assembles a session.Session from a stored agent and the
// stored session record. It instantiates the model via the providers
// registry, resolves the tool registry, and wires persistence directly
// via WithStore so the session loads its history from disk on Run and
// persists every new message back as it lands.
func (s *Server) buildSession(stored *store.Agent, sess *store.Session) (*session.Session, error) {
	return s.buildSessionWithStore(stored, sess, s.store)
}

func (s *Server) buildEphemeralSession(stored *store.Agent, sess *store.Session) (*session.Session, error) {
	return s.buildSessionWithStore(stored, sess, nil)
}

func (s *Server) buildSessionWithStore(stored *store.Agent, sess *store.Session, st store.Store) (*session.Session, error) {
	if stored.ModelRef == "" {
		return nil, fmt.Errorf("model_ref is required when agent has no model_ref")
	}

	modelRef, modelInfo, client, err := s.buildModelClient(stored)
	if err != nil {
		return nil, err
	}

	opts := []session.Option{
		session.WithID(sess.ID),
		session.WithClient(client),
		session.WithModelRef(modelRef, modelInfo),
		session.WithSystem(stored.Instructions),
		session.WithWorkDir(sess.WorkDir),
		session.WithPermissions(s.effectivePermissions(stored)),
		session.WithLogger(s.logger.With("agent_id", stored.ID)),
		session.WithAgentID(stored.ID),
	}
	if st != nil {
		opts = append(opts, session.WithStore(st))
	}
	if s.plugins != nil {
		s.plugins.EnsureWorkDir(context.Background(), sess.WorkDir)
	}
	if tools := s.resolveTools(stored.Tools); len(tools) > 0 {
		opts = append(opts, session.WithTools(tools...))
	}
	if len(stored.OutputSchema) > 0 {
		opts = append(opts, session.WithOutputSchema(&models.OutputSchema{
			Name:   stored.ID,
			Schema: stored.OutputSchema,
			Strict: true,
		}))
	}

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
func (s *Server) buildModelClient(stored *store.Agent) (models.ModelRef, models.ModelInfo, models.Client, error) {
	ref, ok := models.ParseModelRef(stored.ModelRef)
	if !ok {
		return models.ModelRef{}, models.ModelInfo{}, nil, fmt.Errorf("invalid model_ref: %s", stored.ModelRef)
	}
	info, err := s.resolveModelInfo(ref, stored.Options)
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
	return ref, info, provider.NewClientWithCredentials(credentials, s.providers, s.refreshProviderCredential), nil
}

func (s *Server) resolveModelInfo(ref models.ModelRef, options map[string]any) (models.ModelInfo, error) {
	if info, ok := catalog.Get(ref.Provider, ref.ID); ok {
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

// resolveTools maps stored tool name strings to live tool.Tool
// implementations. Unknown names are silently dropped; callers that
// need strict validation should validate at agent-creation time.
func (s *Server) resolveTools(toolNames []string) []tool.Tool {
	builtins := nativeTools()
	if s.plugins != nil {
		for _, t := range s.plugins.Tools() {
			builtins[t.Name()] = t
		}
	}
	if s.mcp != nil {
		for _, t := range s.mcp.Tools() {
			builtins[t.Name()] = t
		}
	}

	var tools []tool.Tool
	for _, name := range toolNames {
		if t, ok := builtins[name]; ok {
			tools = append(tools, t)
		}
	}
	return tools
}

func nativeTools() map[string]tool.Tool {
	return map[string]tool.Tool{
		"apply_patch": tool.NewApplyPatchTool(),
		"bash":        tool.NewBashTool(),
		"read":        tool.NewReadTool(),
		"write":       tool.NewWriteTool(),
		"edit":        tool.NewEditTool(),
		"glob":        tool.NewGlobTool(),
		"grep":        tool.NewGrepTool(),
		"webfetch":    tool.NewWebFetchTool(),
		"websearch":   tool.NewWebSearchTool(),
	}
}
