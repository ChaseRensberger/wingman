package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/trigger"
)

func (s *Server) registerTriggerRoutes() {
	s.registerJSON(http.MethodGet, "/triggers", "listTriggers", "List client triggers", nil, http.StatusOK, []trigger.Trigger{}, s.handleListTriggers)
	s.registerJSON(http.MethodPost, "/triggers", "createTrigger", "Create a trigger", trigger.Config{}, http.StatusCreated, trigger.Trigger{}, s.handleCreateTrigger)
	s.registerJSON(http.MethodPost, "/triggers/preview", "previewTrigger", "Preview the next five scheduled times", trigger.Source{}, http.StatusOK, api.TriggerPreview{}, s.handlePreviewTrigger)
	s.registerJSON(http.MethodGet, "/triggers/occurrences", "listTriggerOccurrences", "List the latest 50 client trigger occurrences", nil, http.StatusOK, []trigger.Occurrence{}, s.handleTriggerOccurrences)
	s.registerJSON(http.MethodGet, "/triggers/{id}", "getTrigger", "Get a trigger", nil, http.StatusOK, trigger.Trigger{}, s.handleGetTrigger)
	s.registerJSON(http.MethodPut, "/triggers/{id}", "updateTrigger", "Replace a trigger definition", api.UpdateTriggerRequest{}, http.StatusOK, trigger.Trigger{}, s.handleUpdateTrigger)
	s.registerJSON(http.MethodPut, "/triggers/{id}/state", "setTriggerState", "Pause or resume a trigger", api.TriggerStateRequest{}, http.StatusOK, trigger.Trigger{}, s.handleSetTriggerState)
	s.registerJSONWithParameters(http.MethodDelete, "/triggers/{id}", "deleteTrigger", "Delete a trigger and its occurrence history; keep its Sessions", nil, http.StatusOK, api.StatusResponse{}, []*huma.Param{{Name: "expected_version", In: "query", Required: true, Schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64"}}}, s.handleDeleteTrigger)
	s.registerJSON(http.MethodPost, "/triggers/{id}/fire", "fireTrigger", "Submit a trigger manually without changing its schedule", api.FireTriggerRequest{}, http.StatusAccepted, trigger.Occurrence{}, s.handleFireTrigger)
	s.registerJSON(http.MethodGet, "/triggers/{id}/occurrences", "listOccurrencesForTrigger", "List the latest 50 occurrences for a trigger", nil, http.StatusOK, []trigger.Occurrence{}, s.handleTriggerOccurrences)
}

func (s *Server) triggerClient(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.triggers == nil {
		s.writeError(w, http.StatusNotImplemented, "triggers require persistent storage")
		return "", false
	}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return clientID, true
}

func (s *Server) triggerForRequest(w http.ResponseWriter, r *http.Request) (trigger.Trigger, bool) {
	clientID, ok := s.triggerClient(w, r)
	if !ok {
		return trigger.Trigger{}, false
	}
	t, err := s.triggers.store.GetTrigger(r.Context(), clientID, chi.URLParam(r, "id"))
	if err != nil {
		s.writeTriggerError(w, err)
		return t, false
	}
	return t, true
}

func (s *Server) writeTriggerError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, trigger.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, trigger.ErrConflict) {
		status = http.StatusConflict
	}
	s.writeError(w, status, err.Error())
}

func (s *Server) validateTriggerConfig(c *trigger.Config, clientID string) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Source.Expression = strings.Join(strings.Fields(c.Source.Expression), " ")
	c.Source.TimeZone = strings.TrimSpace(c.Source.TimeZone)
	if err := c.Validate(time.Now()); err != nil {
		return err
	}
	agent, err := s.store.GetAgent(c.Target.AgentID)
	if err != nil {
		return errors.New("agent not found")
	}
	modelRef := c.Target.ModelRef
	if modelRef == "" {
		modelRef = agent.ModelRef
	}
	if _, ok := models.ParseModelRef(modelRef); !ok {
		return errors.New("select an agent with a model, or supply a valid model_ref")
	}
	workDir, _, err := s.resolveSessionLocation(clientID, c.Target.WorkingDirectory, c.Target.WorkspaceID)
	if err != nil {
		return err
	}
	if c.Target.WorkspaceID == "" {
		c.Target.WorkingDirectory = workDir
	}
	return nil
}

func (s *Server) handleListTriggers(w http.ResponseWriter, r *http.Request) {
	clientID, ok := s.triggerClient(w, r)
	if !ok {
		return
	}
	items, err := s.triggers.store.ListTriggers(r.Context(), clientID)
	if err != nil {
		s.writeTriggerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateTrigger(w http.ResponseWriter, r *http.Request) {
	clientID, ok := s.triggerClient(w, r)
	if !ok {
		return
	}
	var config trigger.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&config); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.validateTriggerConfig(&config, clientID); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveTrigger(w, r, trigger.Trigger{Config: config, ClientID: clientID}, 0)
}

func (s *Server) handleGetTrigger(w http.ResponseWriter, r *http.Request) {
	if t, ok := s.triggerForRequest(w, r); ok {
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) handleUpdateTrigger(w http.ResponseWriter, r *http.Request) {
	t, ok := s.triggerForRequest(w, r)
	if !ok {
		return
	}
	var req api.UpdateTriggerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.ExpectedVersion <= 0 {
		s.writeError(w, http.StatusBadRequest, "a definition and positive expected_version are required")
		return
	}
	if err := s.validateTriggerConfig(&req.Config, t.ClientID); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t.Config = req.Config
	s.saveTrigger(w, r, t, req.ExpectedVersion)
}

func (s *Server) handleSetTriggerState(w http.ResponseWriter, r *http.Request) {
	t, ok := s.triggerForRequest(w, r)
	if !ok {
		return
	}
	var req api.TriggerStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExpectedVersion <= 0 {
		s.writeError(w, http.StatusBadRequest, "enabled and a positive expected_version are required")
		return
	}
	t.Enabled = req.Enabled
	s.saveTrigger(w, r, t, req.ExpectedVersion)
}

func (s *Server) saveTrigger(w http.ResponseWriter, r *http.Request, t trigger.Trigger, expected int64) {
	t.NextFireAt = nil
	if t.Enabled {
		next, err := t.Source.Next(time.Now())
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		t.NextFireAt = &next
	}
	t, err := s.triggers.store.SaveTrigger(r.Context(), t, expected)
	if err != nil {
		s.writeTriggerError(w, err)
		return
	}
	status := http.StatusOK
	if expected == 0 {
		status = http.StatusCreated
	}
	writeJSON(w, status, t)
}

func (s *Server) handleDeleteTrigger(w http.ResponseWriter, r *http.Request) {
	t, ok := s.triggerForRequest(w, r)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
	if err != nil || version <= 0 {
		s.writeError(w, http.StatusBadRequest, "expected_version must be a positive integer")
		return
	}
	if err := s.triggers.store.DeleteTrigger(r.Context(), t.ClientID, t.ID, version); err != nil {
		s.writeTriggerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.StatusResponse{Status: "deleted"})
}

func (s *Server) handleFireTrigger(w http.ResponseWriter, r *http.Request) {
	t, ok := s.triggerForRequest(w, r)
	if !ok {
		return
	}
	var req api.FireTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 200 {
		s.writeError(w, http.StatusBadRequest, "request_id is required and must be at most 200 bytes")
		return
	}
	o, err := s.triggers.fire(r.Context(), t, trigger.Occurrence{Source: "manual", RequestID: req.RequestID, ScheduledAt: time.Now().UTC()}, time.Time{})
	if err != nil {
		s.writeTriggerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, o)
}

func (s *Server) handleTriggerOccurrences(w http.ResponseWriter, r *http.Request) {
	clientID, ok := s.triggerClient(w, r)
	if !ok {
		return
	}
	triggerID := chi.URLParam(r, "id")
	if triggerID != "" {
		if _, ok := s.triggerForRequest(w, r); !ok {
			return
		}
	}
	items, err := s.triggers.store.ListTriggerOccurrences(r.Context(), clientID, triggerID, 50)
	if err != nil {
		s.writeTriggerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handlePreviewTrigger(w http.ResponseWriter, r *http.Request) {
	var source trigger.Source
	if err := json.NewDecoder(r.Body).Decode(&source); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	preview := api.TriggerPreview{Times: []time.Time{}}
	next := time.Now()
	for range 5 {
		var err error
		next, err = source.Next(next)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		preview.Times = append(preview.Times, next)
	}
	writeJSON(w, http.StatusOK, preview)
}
