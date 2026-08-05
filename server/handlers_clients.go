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
	if !s.requireRoot(w, r) {
		return
	}
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req api.CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ID, req.Name = strings.TrimSpace(req.ID), strings.TrimSpace(req.Name)
	if !validClientID(req.ID) || req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "id must start with cli_ and contain lowercase letters, numbers, underscores, or hyphens; name is required")
		return
	}
	if _, err := s.store.EnsureDefaultClient(); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	client, err := s.store.CreateClientWithID(req.ID, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrClientNameExists) || errors.Is(err, store.ErrClientIDExists) {
			s.writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	token, session, err := s.createAuthSession(client.ID, false, "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, api.CreateClientResponse{Client: apiClient(client), Session: apiAuthSession(session), Token: token})
}

func (s *Server) handleRotateClientToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	client, err := s.store.GetClient(chi.URLParam(r, "id"))
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	sessions, err := s.authStore.ListAuthSessions(client.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, session := range sessions {
		if !session.Owner && session.RevokedAt == "" {
			if err := s.authStore.RevokeAuthSession(session.ID); err != nil {
				s.writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	token, session, err := s.createAuthSession(client.ID, false, "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.CreateClientResponse{Client: apiClient(client), Session: apiAuthSession(session), Token: token})
}

func validClientID(id string) bool {
	if !strings.HasPrefix(id, store.PrefixClient) || len(id) == len(store.PrefixClient) {
		return false
	}
	for _, r := range id[len(store.PrefixClient):] {
		if r != '_' && r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
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
	if !s.requireRoot(w, r) {
		return
	}
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
