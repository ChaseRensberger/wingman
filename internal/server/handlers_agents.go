package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"wingman/internal/storage"
)

type CreateAgentRequest struct {
	Name         string   `json:"name"`
	Instructions string   `json:"instructions,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	MaxSteps     int      `json:"max_steps,omitempty"`
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	agent := &storage.Agent{
		Name:         req.Name,
		Instructions: req.Instructions,
		Tools:        req.Tools,
		MaxTokens:    req.MaxTokens,
		Temperature:  req.Temperature,
		MaxSteps:     req.MaxSteps,
	}

	if err := s.store.CreateAgent(agent); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, agent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	agent, err := s.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agent)
}

type UpdateAgentRequest struct {
	Name         *string  `json:"name,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	MaxTokens    *int     `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	MaxSteps     *int     `json:"max_steps,omitempty"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	agent, err := s.store.GetAgent(id)
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
		agent.Name = *req.Name
	}
	if req.Instructions != nil {
		agent.Instructions = *req.Instructions
	}
	if req.Tools != nil {
		agent.Tools = req.Tools
	}
	if req.MaxTokens != nil {
		agent.MaxTokens = *req.MaxTokens
	}
	if req.Temperature != nil {
		agent.Temperature = req.Temperature
	}
	if req.MaxSteps != nil {
		agent.MaxSteps = *req.MaxSteps
	}

	if err := s.store.UpdateAgent(agent); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteAgent(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
