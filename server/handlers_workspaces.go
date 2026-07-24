package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/store"
)

type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type directoryListing struct {
	Path    string           `json:"path"`
	Parent  string           `json:"parent,omitempty"`
	Entries []directoryEntry `json:"entries"`
}

type directoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleListDirectories(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "resolve home directory")
			return
		}
		dir = home
	}
	resolved, err := session.ResolveWorkDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	listing := directoryListing{Path: resolved, Entries: make([]directoryEntry, 0, len(entries))}
	if parent := filepath.Dir(resolved); parent != resolved {
		listing.Parent = parent
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.IsDir() {
			continue
		}
		listing.Entries = append(listing.Entries, directoryEntry{Name: entry.Name(), Path: filepath.Join(resolved, entry.Name())})
	}
	sort.Slice(listing.Entries, func(i, j int) bool {
		return strings.ToLower(listing.Entries[i].Name) < strings.ToLower(listing.Entries[j].Name)
	})
	writeJSON(w, http.StatusOK, listing)
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
	path, err := session.ResolveWorkDir(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" && path != "" {
		name = filepath.Base(path)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required when no directory is set")
		return
	}
	workspace := &store.Workspace{Name: name, Path: path}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspace.ClientID = clientID

	if err := s.store.CreateWorkspace(workspace); err != nil {
		if errors.Is(err, store.ErrWorkspaceNameExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
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
		workspace.Name = strings.TrimSpace(*req.Name)
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
		if errors.Is(err, store.ErrWorkspaceNameExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
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
