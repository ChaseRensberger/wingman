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

func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
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
	id := chi.URLParam(r, "id")

	client, err := s.store.GetClient(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, client)
}
