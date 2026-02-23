package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/internal/storage"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/provider"
	"github.com/chaserensberger/wingman/provider/anthropic"
	"github.com/chaserensberger/wingman/provider/ollama"
	"github.com/chaserensberger/wingman/session"
	"github.com/chaserensberger/wingman/tool"
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
	ToolCalls []session.ToolCallResult `json:"tool_calls"`
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
		agent.WithID(stored.ID),
		agent.WithInstructions(stored.Instructions),
	}

	tools := s.resolveTools(stored.Tools)
	if len(tools) > 0 {
		opts = append(opts, agent.WithTools(tools...))
	}

	if stored.Model != "" {
		p, err := s.buildProvider(stored.Model, stored.Options)
		if err != nil {
			return nil, err
		}
		opts = append(opts, agent.WithProvider(p))
	}

	if stored.OutputSchema != nil {
		opts = append(opts, agent.WithOutputSchema(stored.OutputSchema))
	}

	return agent.New(stored.Name, opts...), nil
}

func (s *Server) buildProvider(model string, opts map[string]any) (provider.Provider, error) {
	// Split "provider/model" into provider ID and model ID
	slashIdx := -1
	for i, c := range model {
		if c == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx < 0 {
		return nil, fmt.Errorf("invalid model format %q: expected \"provider/model\"", model)
	}
	providerID := model[:slashIdx]
	modelID := model[slashIdx+1:]

	auth, err := s.store.GetAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth: %w", err)
	}

	getString := func(key string) string {
		if v, ok := opts[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}
	getInt := func(key string) int {
		if v, ok := opts[key]; ok {
			switch n := v.(type) {
			case int:
				return n
			case float64:
				return int(n)
			}
		}
		return 0
	}
	getFloat64Ptr := func(key string) *float64 {
		if v, ok := opts[key]; ok {
			if f, ok := v.(float64); ok {
				return &f
			}
		}
		return nil
	}

	switch providerID {
	case "anthropic":
		cred := auth.Providers["anthropic"]
		apiKey := cred.Key
		if k := getString("api_key"); k != "" {
			apiKey = k
		}
		if apiKey == "" {
			return nil, fmt.Errorf("anthropic not configured: missing API key")
		}
		acfg := anthropic.Config{
			APIKey:      apiKey,
			Model:       modelID,
			MaxTokens:   getInt("max_tokens"),
			Temperature: getFloat64Ptr("temperature"),
		}
		return anthropic.New(acfg), nil

	case "ollama":
		ocfg := ollama.Config{
			Model:       modelID,
			BaseURL:     getString("base_url"),
			MaxTokens:   getInt("max_tokens"),
			Temperature: getFloat64Ptr("temperature"),
		}
		return ollama.New(ocfg), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", providerID)
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
