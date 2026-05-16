package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/store"
)

type CreateClientRequest struct {
	Name string `json:"name"`
}

// handleCreateClient registers an application or integration consuming the
// Wingman HTTP API. It is attribution/organization, not auth.
func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	client, err := s.store.CreateClient(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, client)
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	clients, err := s.store.ListClients()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []*store.Client{}
	}
	writeJSON(w, http.StatusOK, clients)
}

func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")

	client, err := s.store.GetClient(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, client)
}
