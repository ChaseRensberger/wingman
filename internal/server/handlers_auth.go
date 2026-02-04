package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"wingman/internal/storage"
)

type AuthResponse struct {
	Providers map[string]ProviderInfo `json:"providers"`
	UpdatedAt string                  `json:"updated_at,omitempty"`
}

type ProviderInfo struct {
	Type       string `json:"type"`
	Configured bool   `json:"configured"`
}

func (s *Server) handleGetAuth(w http.ResponseWriter, r *http.Request) {
	auth, err := s.store.GetAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := AuthResponse{
		Providers: make(map[string]ProviderInfo),
		UpdatedAt: auth.UpdatedAt,
	}

	for name, cred := range auth.Providers {
		resp.Providers[name] = ProviderInfo{
			Type:       cred.Type,
			Configured: cred.Key != "" || cred.AccessToken != "",
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type SetAuthRequest struct {
	Providers map[string]storage.AuthCredential `json:"providers"`
}

func (s *Server) handleSetAuth(w http.ResponseWriter, r *http.Request) {
	var req SetAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	auth, err := s.store.GetAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for name, cred := range req.Providers {
		if !isValidProviderName(name) {
			writeError(w, http.StatusBadRequest, "invalid provider name: "+name)
			return
		}
		auth.Providers[name] = cred
	}

	if err := s.store.SetAuth(auth); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func isValidProviderName(name string) bool {
	name = strings.ToLower(name)
	validProviders := []string{"anthropic", "openai", "google", "bedrock", "azure"}
	for _, p := range validProviders {
		if name == p {
			return true
		}
	}
	return true
}
