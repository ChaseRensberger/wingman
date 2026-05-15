package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"

	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
)

type CreateSessionRequest struct {
	Title            string `json:"title,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
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

	workDir, err := session.ResolveWorkDir(req.WorkingDirectory)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &store.Session{
		Title:   title,
		WorkDir: workDir,
	}

	clientID := r.Header.Get("X-Wingman-Client")
	if clientID != "" {
		if _, err := s.store.GetClient(clientID); err != nil {
			writeError(w, http.StatusBadRequest, "client not found: "+clientID)
			return
		}
		sess.ClientID = clientID
	}

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

	clientID := r.Header.Get("X-Wingman-Client")
	if clientID != "" {
		sessions, err = s.store.ListSessionsByClient(clientID)
	} else {
		sessions, err = s.store.ListSessions()
	}
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

	writeJSON(w, http.StatusOK, SessionDetailResponse{
		Session: sess,
		History: history,
	})
}

type SessionDetailResponse struct {
	*store.Session
	History []models.Message `json:"history"`
}

func (s *Server) sessionHistory(ctx context.Context, sessionID string) ([]models.Message, error) {
	storedMsgs, err := s.store.ListMessages(ctx, sessionID)
	if err != nil {
		if err == store.ErrSessionNotFound {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("list messages: %w", err)
	}

	history := make([]models.Message, len(storedMsgs))
	for i, sm := range storedMsgs {
		msg := models.Message{Role: models.Role(sm.Role)}
		if len(sm.MetadataJSON) > 0 {
			var meta models.Meta
			if err := json.Unmarshal(sm.MetadataJSON, &meta); err != nil {
				return nil, fmt.Errorf("unmarshal message metadata: %w", err)
			}
			msg.Metadata = meta
		}
		content := make(models.Content, len(sm.Parts))
		for j, sp := range sm.Parts {
			part, err := models.UnmarshalPart(sp.PayloadJSON)
			if err != nil {
				return nil, fmt.Errorf("unmarshal message part: %w", err)
			}
			content[j] = part
		}
		msg.Content = content
		history[i] = msg
	}
	if history == nil {
		history = []models.Message{}
	}
	return history, nil
}

type UpdateSessionRequest struct {
	Title            *string `json:"title,omitempty"`
	WorkingDirectory *string `json:"working_directory,omitempty"`
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title != nil {
		sess.Title = *req.Title
	}
	if req.WorkingDirectory != nil {
		workDir, err := session.ResolveWorkDir(*req.WorkingDirectory)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sess.WorkDir = workDir
	}

	if err := s.store.UpdateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteSession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
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
	Message      string               `json:"message"`
	OutputSchema *messageOutputSchema `json:"output_schema,omitempty"`
}

type MessageSessionResponse struct {
	Response  string                   `json:"response"`
	ToolCalls []session.ToolCallResult `json:"tool_calls"`
	Usage     models.Usage             `json:"usage"`
	Steps     int                      `json:"steps"`
}

// RunRequest is the body for POST /run. In ephemeral mode agent is
// required and agent_id is rejected. In normal mode either agent_id
// (looked up from the store) or agent (inline spec) is accepted.
type RunRequest struct {
	AgentID          string               `json:"agent_id,omitempty"`
	Agent            *store.Agent         `json:"agent,omitempty"`
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

	runSession, err := s.buildSession(storedAgent, sess)
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

	// Register for abort. Aborting the session via POST /abort cancels
	// runCtx, which propagates through the loop and provider stream;
	// the loop emits a terminal turn with FinishReasonAborted and
	// returns. We still persist whatever history was produced before
	// the cancel because AppendMessage runs synchronously per
	// MessageEvent during the loop.
	runCtx, release := s.aborts.register(id, r.Context())
	defer release()

	result, err := runSession.Run(runCtx, req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	toolCalls := result.ToolCalls
	if toolCalls == nil {
		toolCalls = []session.ToolCallResult{}
	}
	writeJSON(w, http.StatusOK, MessageSessionResponse{
		Response:  result.Response,
		ToolCalls: toolCalls,
		Usage:     result.Usage,
		Steps:     result.Steps,
	})
}

func (s *Server) handleMessageStreamSession(w http.ResponseWriter, r *http.Request) {
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

	runSession, err := s.buildSession(storedAgent, sess)
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

	// Wire shutdown signaling: when Shutdown is called server-wide,
	// shutdownCtx fires and we cancel this request's context so the
	// loop returns and the SSE writer below exits its drain loop.
	// trackInflight registers with the WaitGroup so Shutdown waits for
	// us to actually finish (vs. just signalling).
	done := s.trackInflight()
	defer done()
	go func() {
		select {
		case <-s.ShutdownCtx().Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	// Register for abort. See handleMessageSession for the full
	// rationale; the streaming variant additionally flushes any events
	// the loop emits on its way out (e.g. a final FinishPart with
	// FinishReasonAborted).
	ctx, release := s.aborts.register(id, ctx)
	defer release()

	stream, err := runSession.RunStream(ctx, req.Message)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	for stream.Next() {
		event := stream.Event()
		// Send the full envelope as the SSE data payload. The "event:"
		// line still carries Type so EventSource consumers can filter
		// without parsing JSON, but parsing the data yields a fully
		// self-describing {type, version, data} blob suitable for
		// replay/logging without out-of-band context.
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
	n := s.aborts.abort(id)
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

	if storedAgent.Provider == "" || storedAgent.Model == "" {
		writeError(w, http.StatusBadRequest, "agent must have provider and model configured")
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

	runSession, err := s.buildSession(storedAgent, sess)
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
	if stored.Provider == "" || stored.Model == "" {
		return nil, fmt.Errorf("agent %q has no provider/model configured", stored.ID)
	}

	modelRef, modelInfo, client, err := s.buildModelClient(stored.Provider, stored.Model)
	if err != nil {
		return nil, err
	}

	opts := []session.Option{
		session.WithID(sess.ID),
		session.WithClient(client),
		session.WithModelRef(modelRef, modelInfo),
		session.WithSystem(stored.Instructions),
		session.WithWorkDir(sess.WorkDir),
		session.WithStore(s.store),
		session.WithLogger(s.logger.With("agent_id", stored.ID)),
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

// buildModelClient resolves a model ref and returns a catalog-backed model client.
func (s *Server) buildModelClient(providerID, model string) (models.ModelRef, models.ModelInfo, models.Client, error) {
	ref := models.ModelRef{Provider: providerID, ID: model}
	info, ok := catalog.Get(providerID, model)
	if !ok {
		return models.ModelRef{}, models.ModelInfo{}, nil, fmt.Errorf("unknown model: %s", ref.Ref())
	}
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
	keys := map[string]string{}
	for id, cred := range auth.Providers {
		if cred.Key != "" {
			keys[id] = cred.Key
		}
	}
	if cred, ok := auth.Providers[providerID]; ok && cred.Key != "" {
		keys[providerID] = cred.Key
	}
	return ref, info, provider.NewClient(keys), nil
}

// resolveTools maps stored tool name strings to live tool.Tool
// implementations. Unknown names are silently dropped; callers that
// need strict validation should validate at agent-creation time.
func (s *Server) resolveTools(toolNames []string) []tool.Tool {
	builtins := map[string]tool.Tool{
		"bash":     tool.NewBashTool(),
		"read":     tool.NewReadTool(),
		"write":    tool.NewWriteTool(),
		"edit":     tool.NewEditTool(),
		"glob":     tool.NewGlobTool(),
		"grep":     tool.NewGrepTool(),
		"webfetch": tool.NewWebFetchTool(),
	}

	var tools []tool.Tool
	for _, name := range toolNames {
		if t, ok := builtins[name]; ok {
			tools = append(tools, t)
		}
	}
	return tools
}
