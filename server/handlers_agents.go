package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/store"
)

type CreateAgentRequest struct {
	Name         string         `json:"name"`
	Instructions string         `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	ModelRef     string         `json:"model_ref,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Model        string         `json:"model,omitempty"`
	Options      map[string]any `json:"options,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	provider, model := splitModelRef(req.ModelRef, req.Provider, req.Model)
	a := &store.Agent{
		Name:         req.Name,
		Instructions: req.Instructions,
		Tools:        req.Tools,
		ModelRef:     joinModelRef(provider, model),
		Provider:     provider,
		Model:        model,
		Options:      req.Options,
		OutputSchema: req.OutputSchema,
	}

	if err := s.store.CreateAgent(a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	agents, err := s.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agents == nil {
		agents = []*store.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	a, err := s.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, a)
}

type UpdateAgentRequest struct {
	Name         *string        `json:"name,omitempty"`
	Instructions *string        `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	ModelRef     *string        `json:"model_ref,omitempty"`
	Provider     *string        `json:"provider,omitempty"`
	Model        *string        `json:"model,omitempty"`
	Options      map[string]any `json:"options,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	a, err := s.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.Instructions != nil {
		a.Instructions = *req.Instructions
	}
	if req.Tools != nil {
		a.Tools = req.Tools
	}
	if req.ModelRef != nil || req.Provider != nil || req.Model != nil {
		provider := a.Provider
		model := a.Model
		modelRef := ""
		if req.ModelRef != nil {
			modelRef = *req.ModelRef
		}
		if req.Provider != nil {
			provider = *req.Provider
		}
		if req.Model != nil {
			model = *req.Model
		}
		a.Provider, a.Model = splitModelRef(modelRef, provider, model)
		a.ModelRef = joinModelRef(a.Provider, a.Model)
	}
	if req.Options != nil {
		a.Options = req.Options
	}
	if req.OutputSchema != nil {
		a.OutputSchema = req.OutputSchema
	}

	if err := s.store.UpdateAgent(a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, a)
}

func splitModelRef(modelRef, provider, model string) (string, string) {
	if modelRef == "" {
		return provider, model
	}
	left, right, ok := strings.Cut(modelRef, "/")
	if !ok || left == "" || right == "" {
		return provider, model
	}
	return left, right
}

func joinModelRef(provider, model string) string {
	if provider == "" || model == "" {
		return ""
	}
	return provider + "/" + model
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteAgent(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
