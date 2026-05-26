package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/store"
)

const defaultBaseName = "Wingman"

func (s *Server) ensureDefaultBase(clientID string) (*store.Base, error) {
	var bases []*store.Base
	var err error
	if clientID != "" {
		bases, err = s.store.ListBasesByClient(clientID)
	} else {
		bases, err = s.store.ListBases()
	}
	if err != nil {
		return nil, err
	}
	for _, base := range bases {
		if base.Name == defaultBaseName {
			return base, nil
		}
	}

	path, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("default base path: %w", err)
	}
	base := &store.Base{Name: defaultBaseName, Path: path, ClientID: clientID}
	if err := s.store.CreateBase(base); err != nil {
		return nil, err
	}
	return base, nil
}

type CreateBaseRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleCreateBase(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req CreateBaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	path, err := session.ResolveWorkDir(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	base := &store.Base{Name: req.Name, Path: path}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	base.ClientID = clientID

	if err := s.store.CreateBase(base); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, base)
}

func (s *Server) handleListBases(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var bases []*store.Base
	var err error
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.ensureDefaultBase(clientID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bases, err = s.store.ListBasesByClient(clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bases == nil {
		bases = []*store.Base{}
	}
	writeJSON(w, http.StatusOK, bases)
}

func (s *Server) handleGetBase(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	base, err := s.store.GetBase(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, base)
}

type UpdateBaseRequest struct {
	Name *string `json:"name,omitempty"`
	Path *string `json:"path,omitempty"`
}

func (s *Server) handleUpdateBase(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	base, err := s.store.GetBase(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateBaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil {
		base.Name = *req.Name
	}
	if req.Path != nil {
		path, err := session.ResolveWorkDir(*req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if path == "" {
			writeError(w, http.StatusBadRequest, "path is required")
			return
		}
		base.Path = path
	}
	if base.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.store.UpdateBase(base); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, base)
}

func (s *Server) handleDeleteBase(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	if err := s.store.DeleteBase(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListBaseSessions(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	baseID := chi.URLParam(r, "id")
	if _, err := s.store.GetBase(baseID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	sessions, err := s.store.ListSessionsByBase(baseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []*store.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}
