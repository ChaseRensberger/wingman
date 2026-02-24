package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/fleet"
	"github.com/chaserensberger/wingman/internal/storage"
)

type CreateFleetRequest struct {
	Name        string `json:"name"`
	AgentID     string `json:"agent_id"`
	WorkerCount int    `json:"worker_count,omitempty"`
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

	if _, err := s.store.GetAgent(req.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found: "+req.AgentID)
		return
	}

	f := &storage.Fleet{
		Name:        req.Name,
		AgentID:     req.AgentID,
		WorkerCount: req.WorkerCount,
		WorkDir:     req.WorkDir,
		Status:      storage.FleetStatusStopped,
	}

	if err := s.store.CreateFleet(f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, f)
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
	f, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

type UpdateFleetRequest struct {
	Name        *string `json:"name,omitempty"`
	WorkerCount *int    `json:"worker_count,omitempty"`
	WorkDir     *string `json:"work_dir,omitempty"`
}

func (s *Server) handleUpdateFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	f, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		f.Name = *req.Name
	}
	if req.WorkerCount != nil {
		f.WorkerCount = *req.WorkerCount
	}
	if req.WorkDir != nil {
		f.WorkDir = *req.WorkDir
	}

	if err := s.store.UpdateFleet(f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteFleet(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type FleetTask struct {
	Message      string `json:"message"`
	WorkDir      string `json:"work_dir,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Data         any    `json:"data,omitempty"`
}

type RunFleetRequest struct {
	Tasks []FleetTask `json:"tasks"`
}

type FleetResultResponse struct {
	TaskIndex  int    `json:"task_index"`
	WorkerName string `json:"worker_name"`
	Response   string `json:"response,omitempty"`
	Error      string `json:"error,omitempty"`
	Steps      int    `json:"steps"`
	Data       any    `json:"data,omitempty"`
}

func (s *Server) handleRunFleet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	storedFleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req RunFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Tasks) == 0 {
		writeError(w, http.StatusBadRequest, "tasks is required")
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

	tasks := make([]fleet.Task, len(req.Tasks))
	for i, t := range req.Tasks {
		wd := t.WorkDir
		if wd == "" {
			wd = storedFleet.WorkDir
		}
		tasks[i] = fleet.Task{
			Message:      t.Message,
			WorkDir:      wd,
			Instructions: t.Instructions,
			Data:         t.Data,
		}
	}

	f := fleet.New(fleet.Config{
		Agent:      agentInstance,
		Tasks:      tasks,
		WorkDir:    storedFleet.WorkDir,
		MaxWorkers: storedFleet.WorkerCount,
	})

	results, err := f.Run(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]FleetResultResponse, len(results))
	for i, res := range results {
		r := FleetResultResponse{
			TaskIndex:  res.TaskIndex,
			WorkerName: res.WorkerName,
			Data:       res.Data,
		}
		if res.Error != nil {
			r.Error = res.Error.Error()
		} else if res.Result != nil {
			r.Response = res.Result.Response
			r.Steps = res.Result.Steps
		}
		resp[i] = r
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRunFleetStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	storedFleet, err := s.store.GetFleet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req RunFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Tasks) == 0 {
		writeError(w, http.StatusBadRequest, "tasks is required")
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

	tasks := make([]fleet.Task, len(req.Tasks))
	for i, t := range req.Tasks {
		wd := t.WorkDir
		if wd == "" {
			wd = storedFleet.WorkDir
		}
		tasks[i] = fleet.Task{
			Message:      t.Message,
			WorkDir:      wd,
			Instructions: t.Instructions,
			Data:         t.Data,
		}
	}

	f := fleet.New(fleet.Config{
		Agent:      agentInstance,
		Tasks:      tasks,
		WorkDir:    storedFleet.WorkDir,
		MaxWorkers: storedFleet.WorkerCount,
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	fs, err := f.RunStream(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	for fs.Next() {
		res := fs.Result()
		rr := FleetResultResponse{
			TaskIndex:  res.TaskIndex,
			WorkerName: res.WorkerName,
			Data:       res.Data,
		}
		if res.Error != nil {
			rr.Error = res.Error.Error()
		} else if res.Result != nil {
			rr.Response = res.Result.Response
			rr.Steps = res.Result.Steps
		}

		data, _ := json.Marshal(rr)
		fmt.Fprintf(w, "event: result\ndata: %s\n\n", data)
		flusher.Flush()
	}

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}
