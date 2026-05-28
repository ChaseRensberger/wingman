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

const defaultWorkspaceName = "Wingman"

func (s *Server) ensureDefaultWorkspace(clientID string) (*store.Workspace, error) {
	var workspaces []*store.Workspace
	var err error
	if clientID != "" {
		workspaces, err = s.store.ListWorkspacesByClient(clientID)
	} else {
		workspaces, err = s.store.ListWorkspaces()
	}
	if err != nil {
		return nil, err
	}
	for _, workspace := range workspaces {
		if workspace.Name == defaultWorkspaceName {
			return workspace, nil
		}
	}

	path, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("default workspace path: %w", err)
	}
	workspace := &store.Workspace{Name: defaultWorkspaceName, Path: path, ClientID: clientID}
	if err := s.store.CreateWorkspace(workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}

type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var req CreateWorkspaceRequest
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

	workspace := &store.Workspace{Name: req.Name, Path: path}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspace.ClientID = clientID

	if err := s.store.CreateWorkspace(workspace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var workspaces []*store.Workspace
	var err error
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.ensureDefaultWorkspace(clientID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	workspaces, err = s.store.ListWorkspacesByClient(clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if workspaces == nil {
		workspaces = []*store.Workspace{}
	}
	writeJSON(w, http.StatusOK, workspaces)
}

func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	workspace, err := s.store.GetWorkspace(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

type UpdateWorkspaceRequest struct {
	Name *string `json:"name,omitempty"`
	Path *string `json:"path,omitempty"`
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	workspace, err := s.store.GetWorkspace(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil {
		workspace.Name = *req.Name
	}
	if req.Path != nil {
		path, err := session.ResolveWorkDir(*req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		workspace.Path = path
	}
	if workspace.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.store.UpdateWorkspace(workspace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	if err := s.store.DeleteWorkspace(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListWorkspaceSessions(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	workspaceID := chi.URLParam(r, "id")
	if _, err := s.store.GetWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	sessions, err := s.store.ListSessionsByWorkspace(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []*store.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}
