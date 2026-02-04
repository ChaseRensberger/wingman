package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"wingman/agent"
	"wingman/internal/storage"
	"wingman/models"
	"wingman/provider/claude"
	"wingman/session"
	"wingman/tool"
)

type CreateSessionRequest struct {
	WorkDir string `json:"work_dir,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sess := &storage.Session{
		WorkDir: req.WorkDir,
		History: []models.WingmanMessage{},
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

type RunSessionRequest struct {
	AgentID string `json:"agent_id"`
	Prompt  string `json:"prompt"`
}

type RunSessionResponse struct {
	Response  string                   `json:"response"`
	ToolCalls []session.ToolCallResult `json:"tool_calls,omitempty"`
	Usage     models.WingmanUsage      `json:"usage"`
	Steps     int                      `json:"steps"`
}

func (s *Server) handleRunSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req RunSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
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

	provider, err := s.getProvider("claude")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	agentInstance := s.buildAgent(storedAgent)

	runSession := session.New(
		session.WithAgent(agentInstance),
		session.WithProvider(provider),
		session.WithWorkDir(sess.WorkDir),
	)

	for _, msg := range sess.History {
		runSession.AddMessage(msg)
	}

	result, err := runSession.Run(r.Context(), req.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sess.History = runSession.History()
	if err := s.store.UpdateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RunSessionResponse{
		Response:  result.Response,
		ToolCalls: result.ToolCalls,
		Usage:     result.Usage,
		Steps:     result.Steps,
	})
}

func (s *Server) handleStreamSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req RunSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
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

	provider, err := s.getProvider("claude")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	agentInstance := s.buildAgent(storedAgent)

	runSession := session.New(
		session.WithAgent(agentInstance),
		session.WithProvider(provider),
		session.WithWorkDir(sess.WorkDir),
	)

	for _, msg := range sess.History {
		runSession.AddMessage(msg)
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

	stream, err := runSession.RunStream(ctx, req.Prompt)
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
	sess.History = runSession.History()
	if err := s.store.UpdateSession(sess); err != nil {
		fmt.Fprintf(w, "event: error\ndata: failed to save session\n\n")
		flusher.Flush()
		return
	}

	doneData, _ := json.Marshal(map[string]any{
		"usage": result.Usage,
		"steps": result.Steps,
	})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
	flusher.Flush()
}

func (s *Server) getProvider(name string) (*claude.Client, error) {
	auth, err := s.store.GetAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth: %w", err)
	}

	cred, ok := auth.Providers[name]
	if !ok || cred.Key == "" {
		return nil, fmt.Errorf("provider %s not configured", name)
	}

	return claude.New(claude.Config{
		APIKey: cred.Key,
	}), nil
}

func (s *Server) buildAgent(stored *storage.Agent) *agent.Agent {
	opts := []agent.Option{
		agent.WithInstructions(stored.Instructions),
	}

	if stored.MaxTokens > 0 {
		opts = append(opts, agent.WithMaxTokens(stored.MaxTokens))
	}
	if stored.Temperature != nil {
		opts = append(opts, agent.WithTemperature(*stored.Temperature))
	}
	if stored.MaxSteps > 0 {
		opts = append(opts, agent.WithMaxSteps(stored.MaxSteps))
	}

	tools := s.resolveTools(stored.Tools)
	if len(tools) > 0 {
		opts = append(opts, agent.WithTools(tools...))
	}

	return agent.New(stored.Name, opts...)
}

func (s *Server) resolveTools(toolNames []string) []tool.Tool {
	var tools []tool.Tool

	builtins := map[string]tool.Tool{
		"bash":     tool.NewBashTool(),
		"read":     tool.NewReadTool(),
		"write":    tool.NewWriteTool(),
		"edit":     tool.NewEditTool(),
		"glob":     tool.NewGlobTool(),
		"grep":     tool.NewGrepTool(),
		"webfetch": tool.NewWebFetchTool(),
	}

	for _, name := range toolNames {
		if t, ok := builtins[name]; ok {
			tools = append(tools, t)
		}
	}

	return tools
}
