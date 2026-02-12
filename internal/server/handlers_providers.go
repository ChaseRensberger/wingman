package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"wingman/internal/models_dev"
	"wingman/internal/storage"
	"wingman/provider"
)

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := provider.List()
	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	meta, err := provider.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, meta)
}

type ProvidersAuthResponse struct {
	Providers map[string]ProviderAuthInfo `json:"providers"`
	UpdatedAt string                      `json:"updated_at,omitempty"`
}

type ProviderAuthInfo struct {
	Type       string `json:"type"`
	Configured bool   `json:"configured"`
}

func (s *Server) handleGetProvidersAuth(w http.ResponseWriter, r *http.Request) {
	auth, err := s.store.GetAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := ProvidersAuthResponse{
		Providers: make(map[string]ProviderAuthInfo),
		UpdatedAt: auth.UpdatedAt,
	}

	for name, cred := range auth.Providers {
		resp.Providers[name] = ProviderAuthInfo{
			Type:       cred.Type,
			Configured: cred.Key != "" || cred.AccessToken != "",
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type SetProvidersAuthRequest struct {
	Providers map[string]storage.AuthCredential `json:"providers"`
}

func (s *Server) handleSetProvidersAuth(w http.ResponseWriter, r *http.Request) {
	var req SetProvidersAuthRequest
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
		if !provider.IsValid(name) {
			writeError(w, http.StatusBadRequest, "unknown provider: "+name)
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

func (s *Server) handleDeleteProviderAuth(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")

	if !provider.IsValid(providerName) {
		writeError(w, http.StatusBadRequest, "unknown provider: "+providerName)
		return
	}

	auth, err := s.store.GetAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, exists := auth.Providers[providerName]; !exists {
		writeError(w, http.StatusNotFound, "provider not configured: "+providerName)
		return
	}

	delete(auth.Providers, providerName)

	if err := s.store.SetAuth(auth); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListProviderModels(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if !provider.IsValid(name) {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	models, err := models_dev.GetModels(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models)
}

func (s *Server) handleGetProviderModel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	modelID := chi.URLParam(r, "model")

	if !provider.IsValid(name) {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	model, err := models_dev.GetModel(name, modelID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model)
}
