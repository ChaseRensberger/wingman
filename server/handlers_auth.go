package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
)

const authSessionLifetime = 30 * 24 * time.Hour

func randomCredential() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func apiAuthSession(session *store.AuthSession) api.AuthSession {
	return api.AuthSession{ID: session.ID, ClientID: session.ClientID, Owner: session.Owner, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt, RevokedAt: session.RevokedAt}
}

func (s *Server) handleListAuthSessions(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	clientID := r.URL.Query().Get("client_id")
	if clientID != "" {
		s.writeAuthSessions(w, clientID)
		return
	}
	clients, err := s.authStore.ListClients()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sessions := make([]api.AuthSession, 0)
	for _, client := range clients {
		listed, err := s.authStore.ListAuthSessions(client.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, session := range listed {
			sessions = append(sessions, apiAuthSession(session))
		}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) writeAuthSessions(w http.ResponseWriter, clientID string) {
	sessions, err := s.authStore.ListAuthSessions(clientID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := make([]api.AuthSession, len(sessions))
	for i, session := range sessions {
		response[i] = apiAuthSession(session)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleRevokeAuthSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	if err := s.authStore.RevokeAuthSession(chi.URLParam(r, "id")); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.StatusResponse{Status: "ok"})
}
