package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/chaserensberger/wingman/internal/storage"
)

type runFormationRequest struct {
	Inputs    map[string]any `json:"inputs"`
	Overrides map[string]any `json:"overrides,omitempty"`
}

type runFormationResponse struct {
	Status    string                    `json:"status"`
	Outputs   map[string]map[string]any `json:"outputs"`
	Stats     formationRunStats         `json:"stats"`
	Artifacts []map[string]any          `json:"artifacts"`
}

func (s *Server) handleCreateFormation(w http.ResponseWriter, r *http.Request) {
	definition, err := decodeFormationDefinition(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	compiled, err := compileAndValidateDefinition(definition)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	f := &storage.Formation{
		Name:       compiled.Name,
		Version:    compiled.Version,
		Definition: definition,
	}

	if err := s.store.CreateFormation(f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleListFormations(w http.ResponseWriter, r *http.Request) {
	formations, err := s.store.ListFormations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if formations == nil {
		formations = []*storage.Formation{}
	}
	writeJSON(w, http.StatusOK, formations)
}

func (s *Server) handleGetFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleUpdateFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	definition, err := decodeFormationDefinition(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	compiled, err := compileAndValidateDefinition(definition)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	f.Name = compiled.Name
	f.Version = compiled.Version
	f.Definition = definition

	if err := s.store.UpdateFormation(f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteFormation(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleExportFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}

	switch format {
	case "yaml", "yml":
		b, err := yaml.Marshal(f.Definition)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode yaml")
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	default:
		writeJSON(w, http.StatusOK, f.Definition)
	}
}

func (s *Server) handleRunFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	compiled, err := compileAndValidateDefinition(f.Definition)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid formation definition: "+err.Error())
		return
	}

	var req runFormationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.runFormation(r.Context(), compiled, req.Inputs, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, runFormationResponse{
		Status:    "ok",
		Outputs:   result.Outputs,
		Stats:     result.Stats,
		Artifacts: []map[string]any{},
	})
}

func (s *Server) handleRunFormationStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	compiled, err := compileAndValidateDefinition(f.Definition)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid formation definition: "+err.Error())
		return
	}

	var req runFormationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	sink := func(e formationEvent) {
		b, _ := json.Marshal(e)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
		flusher.Flush()
	}

	_, _ = s.runFormation(r.Context(), compiled, req.Inputs, sink)
}
