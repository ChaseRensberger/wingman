package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/macro"
)

func (s *Server) handleListSessionMacros(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sess, ok := s.authorizeSessionForRequest(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if sess.WorkDir == "" {
		writeJSON(w, http.StatusOK, []api.Macro{})
		return
	}
	macros, err := macro.Discover(sess.WorkDir)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "discover macros: "+err.Error())
		return
	}
	result := make([]api.Macro, len(macros))
	for i, definition := range macros {
		result[i] = api.Macro{
			ID: definition.ID, Description: definition.Description, AgentID: definition.AgentID, ModelRef: definition.ModelRef,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRunSessionMacro(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.writeError(w, http.StatusNotImplemented, "persistence is disabled; use POST /run for ephemeral runs")
		return
	}
	sess, ok := s.authorizeSessionForRequest(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if sess.WorkDir == "" {
		s.writeError(w, http.StatusBadRequest, "session has no working directory")
		return
	}
	var req api.MacroSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.MacroID) == "" {
		s.writeError(w, http.StatusBadRequest, "macro_id is required")
		return
	}
	macros, err := macro.Discover(sess.WorkDir)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "discover macros: "+err.Error())
		return
	}
	var definition *macro.Macro
	for i := range macros {
		if macros[i].ID == req.MacroID {
			definition = &macros[i]
			break
		}
	}
	if definition == nil {
		s.writeError(w, http.StatusNotFound, "macro not found: "+req.MacroID)
		return
	}
	agentID := req.AgentID
	if definition.AgentID != "" {
		agentID = definition.AgentID
	}
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent_id is required when the macro has no agent")
		return
	}
	modelRef, modelRoute := req.ModelRef, req.ModelRoute
	if definition.ModelRef != "" {
		modelRef, modelRoute = definition.ModelRef, nil
	}
	s.admitSessionMessage(w, r, sess, api.MessageSessionRequest{
		RequestID: req.RequestID, AgentID: agentID, ModelRef: modelRef, ModelRoute: modelRoute,
		Message: macro.Expand(*definition, req.Arguments), OutputSchema: req.OutputSchema,
	})
}
