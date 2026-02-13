package server

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/actor"
	"github.com/chaserensberger/wingman/internal/storage"
)

type fleetRuntime struct {
	fleet    *actor.Fleet
	storedID string
}

var (
	runningFleets   = make(map[string]*fleetRuntime)
	runningFleetsMu sync.RWMutex
)

type CreateFleetRequest struct {
	Name        string `json:"name"`
	AgentID     string `json:"agent_id"`
	WorkerCount int    `json:"worker_count"`
	WorkDir     string `json:"work_dir,omitempty"`
}

func (s *Server) handleCreateFleet(w http.ResponseWriter, r *http.Request) {
	var req CreateFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if req.WorkerCount <= 0 {
		req.WorkerCount = 3
	}

	if _, err := s.store.GetAgent(req.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
		return
	}

	fleet := &storage.Fleet{
		Name:        req.Name,
		AgentID:     req.AgentID,
		WorkerCount: req.WorkerCount,
		WorkDir:     req.WorkDir,
		Status:      storage.FleetStatusStopped,
	}

	if err := s.store.CreateFleet(fleet); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, fleet)
}

func (s *Server) handleListFleets(w http.ResponseWriter, r *http.Request) {
	fleets, err := s.store.ListFleets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if fleets == nil {
		fleets = []*storage.Fleet{}
	}
	writeJSON(w, http.StatusOK, fleets)
}

func (s *Server) handleGetFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	fleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, fleet)
}

type UpdateFleetRequest struct {
	Name        *string `json:"name,omitempty"`
	WorkerCount *int    `json:"worker_count,omitempty"`
	WorkDir     *string `json:"work_dir,omitempty"`
}

func (s *Server) handleUpdateFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	fleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if fleet.Status == storage.FleetStatusRunning {
		writeError(w, http.StatusConflict, "cannot update running fleet")
		return
	}

	var req UpdateFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		fleet.Name = *req.Name
	}
	if req.WorkerCount != nil {
		fleet.WorkerCount = *req.WorkerCount
	}
	if req.WorkDir != nil {
		fleet.WorkDir = *req.WorkDir
	}

	if err := s.store.UpdateFleet(fleet); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, fleet)
}

func (s *Server) handleDeleteFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	runningFleetsMu.RLock()
	if _, running := runningFleets[id]; running {
		runningFleetsMu.RUnlock()
		writeError(w, http.StatusConflict, "cannot delete running fleet, stop it first")
		return
	}
	runningFleetsMu.RUnlock()

	if err := s.store.DeleteFleet(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleStartFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	runningFleetsMu.Lock()
	defer runningFleetsMu.Unlock()

	if _, running := runningFleets[id]; running {
		writeError(w, http.StatusConflict, "fleet already running")
		return
	}

	storedFleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	storedAgent, err := s.store.GetAgent(storedFleet.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found: "+storedFleet.AgentID)
		return
	}

	agentInstance, err := s.buildAgent(storedAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fleet := actor.NewFleet(actor.FleetConfig{
		WorkerCount: storedFleet.WorkerCount,
		Agent:       agentInstance,
		WorkDir:     storedFleet.WorkDir,
	})

	runningFleets[id] = &fleetRuntime{
		fleet:    fleet,
		storedID: id,
	}

	storedFleet.Status = storage.FleetStatusRunning
	if err := s.store.UpdateFleet(storedFleet); err != nil {
		fleet.Shutdown()
		delete(runningFleets, id)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, storedFleet)
}

func (s *Server) handleStopFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	runningFleetsMu.Lock()
	defer runningFleetsMu.Unlock()

	runtime, running := runningFleets[id]
	if !running {
		writeError(w, http.StatusConflict, "fleet not running")
		return
	}

	runtime.fleet.Shutdown()
	delete(runningFleets, id)

	storedFleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	storedFleet.Status = storage.FleetStatusStopped
	if err := s.store.UpdateFleet(storedFleet); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, storedFleet)
}

type SubmitFleetRequest struct {
	Messages []string `json:"messages"`
}

type SubmitFleetResponse struct {
	Submitted int `json:"submitted"`
}

func (s *Server) handleSubmitFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	runningFleetsMu.RLock()
	runtime, running := runningFleets[id]
	runningFleetsMu.RUnlock()

	if !running {
		writeError(w, http.StatusConflict, "fleet not running, start it first")
		return
	}

	var req SubmitFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required")
		return
	}

	if err := runtime.fleet.SubmitAll(req.Messages); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SubmitFleetResponse{
		Submitted: len(req.Messages),
	})
}
