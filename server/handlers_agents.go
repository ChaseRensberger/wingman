package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

const agentOptionModelRoute = "model_route"

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req api.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.validateAgentTools(r.Context(), req.Tools); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	a := &store.Agent{
		Name:         req.Name,
		Instructions: req.Instructions,
		Tools:        req.Tools,
		Permissions:  req.Permissions,
		ModelRef:     req.ModelRef,
		Options:      req.Options,
		OutputSchema: req.OutputSchema,
	}
	setAgentModelRoute(a, req.ModelRoute)

	if err := s.store.CreateAgent(a); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiAgent(a))
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	agents, err := s.store.ListAgents()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiAgents(agents))
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	a, err := s.store.GetAgent(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiAgent(a))
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	a, err := s.store.GetAgent(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req api.UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.Instructions != nil {
		a.Instructions = *req.Instructions
	}
	if req.Tools != nil {
		if err := s.validateAgentTools(r.Context(), req.Tools); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		a.Tools = req.Tools
	}
	if req.Permissions != nil {
		a.Permissions = req.Permissions
	}
	if req.ModelRef != nil {
		a.ModelRef = *req.ModelRef
	}
	if req.Options != nil {
		a.Options = req.Options
	}
	setAgentModelRoute(a, req.ModelRoute)
	if req.OutputSchema != nil {
		a.OutputSchema = req.OutputSchema
	}

	if err := s.store.UpdateAgent(a); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiAgent(a))
}

func (s *Server) validateAgentTools(ctx context.Context, names []string) error {
	scope, release, err := s.executionScope(ctx, "")
	if err != nil {
		return err
	}
	defer release()
	_, err = s.resolveTools(scope, names)
	return err
}

func setAgentModelRoute(a *store.Agent, route *models.ModelInfo) {
	if route == nil {
		return
	}
	if a.Options == nil {
		a.Options = map[string]any{}
	}
	a.Options[agentOptionModelRoute] = route
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteAgent(id); err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, api.StatusResponse{Status: "deleted"})
}
