package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
)

// handleCreateClient registers an application or integration consuming the
// Wingman HTTP API. It is attribution/organization, not auth.
func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req api.CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := s.store.EnsureDefaultClient(); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	client, err := s.store.CreateClient(req.Name)
	if err != nil {
		if errors.Is(err, store.ErrClientNameExists) {
			s.writeError(w, http.StatusConflict, "client name already exists")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiClient(client))
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	if _, err := s.store.EnsureDefaultClient(); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	clients, err := s.store.ListClients()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiClients(clients))
}

func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	client, err := s.store.GetClient(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiClient(client))
}
