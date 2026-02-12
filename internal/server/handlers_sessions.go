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
	"wingman/provider"
	"wingman/provider/anthropic"
	"wingman/provider/ollama"
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

type MessageSessionRequest struct {
	AgentID string `json:"agent_id"`
	Message string `json:"message"`
}

type MessageSessionResponse struct {
	Response  string                   `json:"response"`
	ToolCalls []session.ToolCallResult `json:"tool_calls,omitempty"`
	Usage     models.WingmanUsage      `json:"usage"`
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

	agentInstance, err := s.buildAgent(storedAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runSession := session.New(
		session.WithAgent(agentInstance),
		session.WithWorkDir(sess.WorkDir),
	)

	for _, msg := range sess.History {
		runSession.AddMessage(msg)
	}

	result, err := runSession.Run(r.Context(), req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sess.History = runSession.History()
	if err := s.store.UpdateSession(sess); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, MessageSessionResponse{
		Response:  result.Response,
		ToolCalls: result.ToolCalls,
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

	agentInstance, err := s.buildAgent(storedAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runSession := session.New(
		session.WithAgent(agentInstance),
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

func (s *Server) buildAgent(stored *storage.Agent) (*agent.Agent, error) {
	opts := []agent.Option{
		agent.WithInstructions(stored.Instructions),
	}

	tools := s.resolveTools(stored.Tools)
	if len(tools) > 0 {
		opts = append(opts, agent.WithTools(tools...))
	}

	if stored.Provider != nil {
		p, err := s.buildProvider(stored.Provider)
		if err != nil {
			return nil, err
		}
		opts = append(opts, agent.WithProvider(p))
	}

	return agent.New(stored.Name, opts...), nil
}

func (s *Server) buildProvider(cfg *storage.ProviderConfig) (provider.Provider, error) {
	auth, err := s.store.GetAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth: %w", err)
	}

	switch cfg.ID {
	case "anthropic":
		cred := auth.Providers["anthropic"]
		if cred.Key == "" {
			return nil, fmt.Errorf("anthropic not configured: missing API key")
		}
		acfg := anthropic.Config{
			APIKey: cred.Key,
			Model:  cfg.Model,
		}
		if cfg.MaxTokens > 0 {
			acfg.MaxTokens = cfg.MaxTokens
		}
		if cfg.Temperature != nil {
			acfg.Temperature = cfg.Temperature
		}
		return anthropic.New(acfg), nil

	case "ollama":
		cred := auth.Providers["ollama"]
		ocfg := ollama.Config{
			BaseURL: cred.AccessToken,
			Model:   cfg.Model,
		}
		if cfg.MaxTokens > 0 {
			ocfg.MaxTokens = cfg.MaxTokens
		}
		if cfg.Temperature != nil {
			ocfg.Temperature = cfg.Temperature
		}
		return ollama.New(ocfg), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.ID)
	}
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
