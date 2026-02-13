package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"wingman/internal/storage"
)

type CreateFormationRequest struct {
	Name    string                  `json:"name"`
	WorkDir string                  `json:"work_dir,omitempty"`
	Roles   []storage.FormationRole `json:"roles"`
	Edges   []storage.FormationEdge `json:"edges,omitempty"`
}

func (s *Server) handleCreateFormation(w http.ResponseWriter, r *http.Request) {
	var req CreateFormationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Roles) == 0 {
		writeError(w, http.StatusBadRequest, "roles is required")
		return
	}

	for _, role := range req.Roles {
		if _, err := s.store.GetAgent(role.AgentID); err != nil {
			writeError(w, http.StatusNotFound, "agent not found for role "+role.Name+": "+role.AgentID)
			return
		}
	}

	formation := &storage.Formation{
		Name:    req.Name,
		WorkDir: req.WorkDir,
		Roles:   req.Roles,
		Edges:   req.Edges,
		Status:  storage.FormationStatusStopped,
	}

	if err := s.store.CreateFormation(formation); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, formation)
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

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, formation)
}

type UpdateFormationRequest struct {
	Name    *string                 `json:"name,omitempty"`
	WorkDir *string                 `json:"work_dir,omitempty"`
	Roles   []storage.FormationRole `json:"roles,omitempty"`
	Edges   []storage.FormationEdge `json:"edges,omitempty"`
}

func (s *Server) handleUpdateFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if formation.Status == storage.FormationStatusRunning {
		writeError(w, http.StatusConflict, "cannot update running formation")
		return
	}

	var req UpdateFormationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		formation.Name = *req.Name
	}
	if req.WorkDir != nil {
		formation.WorkDir = *req.WorkDir
	}
	if req.Roles != nil {
		for _, role := range req.Roles {
			if _, err := s.store.GetAgent(role.AgentID); err != nil {
				writeError(w, http.StatusNotFound, "agent not found for role "+role.Name+": "+role.AgentID)
				return
			}
		}
		formation.Roles = req.Roles
	}
	if req.Edges != nil {
		formation.Edges = req.Edges
	}

	if err := s.store.UpdateFormation(formation); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, formation)
}

func (s *Server) handleDeleteFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if formation.Status == storage.FormationStatusRunning {
		writeError(w, http.StatusConflict, "cannot delete running formation, stop it first")
		return
	}

	if err := s.store.DeleteFormation(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleStartFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if formation.Status == storage.FormationStatusRunning {
		writeError(w, http.StatusConflict, "formation already running")
		return
	}

	writeError(w, http.StatusNotImplemented, "formation runtime not yet implemented")
}

func (s *Server) handleStopFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if formation.Status != storage.FormationStatusRunning {
		writeError(w, http.StatusConflict, "formation not running")
		return
	}

	writeError(w, http.StatusNotImplemented, "formation runtime not yet implemented")
}

func (s *Server) handleMessageFormation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	formation, err := s.store.GetFormation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if formation.Status != storage.FormationStatusRunning {
		writeError(w, http.StatusConflict, "formation not running, start it first")
		return
	}

	writeError(w, http.StatusNotImplemented, "formation runtime not yet implemented")
}
