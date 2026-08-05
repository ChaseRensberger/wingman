package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
)

const (
	enrollmentLifetime  = 5 * time.Minute
	authSessionLifetime = 30 * 24 * time.Hour
)

type enrollmentManager struct {
	mu          sync.Mutex
	enrollments map[string]enrollment
}

type enrollment struct {
	clientID  string
	expiresAt time.Time
}

func newEnrollmentManager() *enrollmentManager {
	return &enrollmentManager{enrollments: make(map[string]enrollment)}
}

func (m *enrollmentManager) create(clientID string, now time.Time) (string, time.Time, error) {
	credential, err := randomCredential()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.Add(enrollmentLifetime)
	m.mu.Lock()
	m.enrollments[tokenHash(credential)] = enrollment{clientID: clientID, expiresAt: expiresAt}
	m.mu.Unlock()
	return credential, expiresAt, nil
}

func (m *enrollmentManager) consume(credential string, now time.Time) (string, bool) {
	clientID, _, ok := m.consumeForRedemption(credential, now)
	return clientID, ok
}

func (m *enrollmentManager) consumeForRedemption(credential string, now time.Time) (string, time.Time, bool) {
	hash := tokenHash(credential)
	m.mu.Lock()
	enrollment, ok := m.enrollments[hash]
	if ok {
		delete(m.enrollments, hash)
	}
	m.mu.Unlock()
	return enrollment.clientID, enrollment.expiresAt, ok && enrollment.expiresAt.After(now)
}

func (m *enrollmentManager) restore(credential, clientID string, expiresAt time.Time, now time.Time) {
	if !expiresAt.After(now) {
		return
	}
	m.mu.Lock()
	m.enrollments[tokenHash(credential)] = enrollment{clientID: clientID, expiresAt: expiresAt}
	m.mu.Unlock()
}

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

func (s *Server) handleCreateEnrollment(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req api.CreateEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		clientID = store.DefaultClientID
	}
	if _, err := s.authStore.GetClient(clientID); err != nil {
		s.writeError(w, http.StatusBadRequest, "client not found: "+clientID)
		return
	}
	credential, expiresAt, err := s.enrollments.create(clientID, time.Now().UTC())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, api.EnrollmentResponse{Credential: credential, ClientID: clientID, ExpiresAt: expiresAt.Format(time.RFC3339Nano)})
}

func (s *Server) handleRedeemEnrollment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req api.RedeemEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = api.AuthSessionModeCookie
	}
	if mode != api.AuthSessionModeCookie && mode != api.AuthSessionModeBearer {
		s.writeError(w, http.StatusBadRequest, "mode must be cookie or bearer")
		return
	}
	now := time.Now().UTC()
	clientID, expiresAt, ok := s.enrollments.consumeForRedemption(req.Credential, now)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "invalid or expired enrollment credential")
		return
	}
	token, session, err := s.createAuthSession(clientID, false)
	if err != nil {
		s.enrollments.restore(req.Credential, clientID, expiresAt, now)
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response := api.RedeemEnrollmentResponse{Session: apiAuthSession(session)}
	if mode == api.AuthSessionModeCookie {
		s.setAuthSessionCookie(w, r, token)
	} else {
		response.Token = token
	}
	writeJSON(w, http.StatusOK, response)
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
