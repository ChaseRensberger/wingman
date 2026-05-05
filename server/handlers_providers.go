package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers"
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
			Configured: cred.Key != "",
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type SetProvidersAuthRequest struct {
	Providers map[string]store.AuthCredential `json:"providers"`
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

// ModelDTO is the API response shape for a single model. It exposes the
// normalized models.ModelInfo fields rather than the raw catalog schema,
// which changes frequently and contains internal pricing/limit details that
// are not part of the public API contract.
type ModelDTO struct {
	Provider          string  `json:"provider"`
	ID                string  `json:"id"`
	ContextWindow     int     `json:"context_window,omitempty"`
	MaxOutput         int     `json:"max_output,omitempty"`
	Tools             bool    `json:"tools"`
	Images            bool    `json:"images"`
	Reasoning         bool    `json:"reasoning"`
	StructuredOutput  bool    `json:"structured_output"`
	InputCostPerMTok  float64 `json:"input_cost_per_mtok,omitempty"`
	OutputCostPerMTok float64 `json:"output_cost_per_mtok,omitempty"`
}

func modelToDTO(info models.ModelInfo) ModelDTO {
	return ModelDTO{
		Provider:          info.Provider,
		ID:                info.ID,
		ContextWindow:     info.ContextWindow,
		MaxOutput:         info.MaxOutput,
		Tools:             info.Capabilities.Tools,
		Images:            info.Capabilities.Images,
		Reasoning:         info.Capabilities.Reasoning,
		StructuredOutput:  info.Capabilities.StructuredOutput,
		InputCostPerMTok:  info.InputCostPerMTok,
		OutputCostPerMTok: info.OutputCostPerMTok,
	}
}

func (s *Server) handleListProviderModels(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if !provider.IsValid(name) {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	rawModels, ok := catalog.GetModels(name)
	if !ok {
		writeError(w, http.StatusNotFound, "no models for provider: "+name)
		return
	}

	dtos := make(map[string]ModelDTO, len(rawModels))
	for id := range rawModels {
		if info, ok := catalog.Get(name, id); ok {
			dtos[id] = modelToDTO(info)
		}
	}

	writeJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleGetProviderModel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	modelID := chi.URLParam(r, "model")

	if !provider.IsValid(name) {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	info, ok := catalog.Get(name, modelID)
	if !ok {
		writeError(w, http.StatusNotFound, "model not found: "+name+"/"+modelID)
		return
	}

	writeJSON(w, http.StatusOK, modelToDTO(info))
}
