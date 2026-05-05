package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/wingagent/session"
	"github.com/chaserensberger/wingman/wingagent/storage"
	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
	"github.com/chaserensberger/wingman/wingmodels/providers"

	_ "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
	_ "github.com/chaserensberger/wingman/wingmodels/providers/ollama"
)

type CreateSessionRequest struct {
	Title   string `json:"title,omitempty"`
	WorkDir string `json:"work_dir,omitempty"`
}

// defaultSessionTitle is the placeholder applied when a session is
// created without an explicit title. The frontend treats this string
// as the canonical "untitled" label so users see the same value
// whether the title is empty or this default.
const defaultSessionTitle = "New session"

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := req.Title
	if title == "" {
		title = defaultSessionTitle
	}

	sess := &storage.Session{
		Title:   title,
		WorkDir: req.WorkDir,
		History: []wingmodels.Message{},
	}

	if err := s.store.CreateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []*storage.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

type UpdateSessionRequest struct {
	Title   *string `json:"title,omitempty"`
	WorkDir *string `json:"work_dir,omitempty"`
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
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
	if req.WorkDir != nil {
		sess.WorkDir = *req.WorkDir
	}

	if err := s.store.UpdateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteSession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type MessageSessionRequest struct {
	AgentID string `json:"agent_id"`
	Message string `json:"message"`
}

type MessageSessionResponse struct {
	Response  string                   `json:"response"`
	ToolCalls []session.ToolCallResult `json:"tool_calls"`
	Usage     wingmodels.Usage         `json:"usage"`
	Steps     int                      `json:"steps"`
}

func (s *Server) handleMessageSession(w http.ResponseWriter, r *http.Request) {
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

// buildSession assembles a session.Session from a stored agent and the
// stored session record. It instantiates the model via the providers
// registry, resolves the tool registry, and installs the storage plugin
// so the session loads its history from disk on Run and persists every
// new message back as it lands.
func (s *Server) buildSession(stored *storage.Agent, sess *storage.Session) (*session.Session, error) {
	if stored.Provider == "" || stored.Model == "" {
		return nil, fmt.Errorf("agent %q has no provider/model configured", stored.ID)
	}

	model, err := s.buildModel(stored.Provider, stored.Model, stored.Options)
	if err != nil {
		return nil, err
	}

	opts := []session.Option{
		session.WithModel(model),
		session.WithSystem(stored.Instructions),
		session.WithWorkDir(sess.WorkDir),
		// The storage plugin packages both sides of persistence:
		// BeforeRun loads sess.History from the store, and a sink
		// listening for loop.MessageEvent appends each completed
		// message back. Replacing the prior explicit AddMessage
		// replay + WithMessageSink wiring with a single plugin call
		// removes the chance of one side being installed without the
		// other, and keeps storage out of the SDK and loop core.
		session.WithPlugin(storage.NewPlugin(s.store, sess.ID)),
	}
	if tools := s.resolveTools(stored.Tools); len(tools) > 0 {
		opts = append(opts, session.WithTools(tools...))
	}
	if len(stored.OutputSchema) > 0 {
		opts = append(opts, session.WithOutputSchema(&wingmodels.OutputSchema{
			Name:   stored.ID,
			Schema: stored.OutputSchema,
			Strict: true,
		}))
	}

	return session.New(opts...), nil
}

// buildModel instantiates a wingmodels.Model from the providers registry.
// It merges the stored options with the model name and any API key from
// the auth store.
func (s *Server) buildModel(providerID, model string, opts map[string]any) (wingmodels.Model, error) {
	merged := make(map[string]any, len(opts)+2)
	for k, v := range opts {
		merged[k] = v
	}
	merged["model"] = model

	auth, err := s.store.GetAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to load auth: %w", err)
	}
	if cred, ok := auth.Providers[providerID]; ok && cred.Key != "" {
		merged["api_key"] = cred.Key
	}

	m, err := provider.New(providerID, merged)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate provider %q: %w", providerID, err)
	}
	return m, nil
}

// resolveTools maps stored tool name strings to live tool.Tool
// implementations. Unknown names are silently dropped; callers that
// need strict validation should validate at agent-creation time.
func (s *Server) resolveTools(toolNames []string) []tool.Tool {
	builtins := map[string]tool.Tool{
		"bash":              tool.NewBashTool(),
		"read":              tool.NewReadTool(),
		"write":             tool.NewWriteTool(),
		"edit":              tool.NewEditTool(),
		"glob":              tool.NewGlobTool(),
		"grep":              tool.NewGrepTool(),
		"webfetch":          tool.NewWebFetchTool(),
		"perplexity_search": tool.NewPerplexityTool(),
	}

	var tools []tool.Tool
	for _, name := range toolNames {
		if t, ok := builtins[name]; ok {
			tools = append(tools, t)
		}
	}
	return tools
}
