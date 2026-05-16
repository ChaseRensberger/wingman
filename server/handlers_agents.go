package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/store"
)

type CreateAgentRequest struct {
	Name         string         `json:"name"`
	Instructions string         `json:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	ModelRef     string         `json:"model_ref,omitempty"`
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

	a := &store.Agent{
		Name:         req.Name,
		Instructions: req.Instructions,
		Tools:        req.Tools,
		ModelRef:     req.ModelRef,
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
	if req.ModelRef != nil {
		a.ModelRef = *req.ModelRef
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
