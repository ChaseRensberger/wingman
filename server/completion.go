package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleReplySessionRunCompletion(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id, runID := chi.URLParam(r, "id"), chi.URLParam(r, "runID")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	var req api.CompletionReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := (run.CompletionDecision{Action: req.Decision.Action, Feedback: req.Decision.Feedback}).Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current, err := s.store.GetSessionRun(r.Context(), id, runID)
	if errors.Is(err, store.ErrSessionRunNotFound) {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !current.Completion.Remote {
		s.writeError(w, http.StatusConflict, "run does not request remote validation")
		return
	}
	var candidate *store.Completion
	for _, c := range current.Completions {
		if c.CandidateID == req.CandidateID {
			candidate = &c
			break
		}
	}
	if candidate == nil {
		s.writeError(w, http.StatusConflict, "completion candidate is stale or unknown")
		return
	}
	// Only remote-waiting candidates or an identical committed reply can be resolved here.
	if candidate.Status != "waiting" && (candidate.Status != req.Decision.Action || candidate.Feedback != req.Decision.Feedback) {
		s.writeError(w, http.StatusConflict, "completion candidate is not awaiting a reply")
		return
	}
	candidate.Status, candidate.Feedback = req.Decision.Action, req.Decision.Feedback
	transition, err := s.store.UpdateSessionRunCompletion(r.Context(), store.CompletionUpdate{SessionID: id, RunID: runID, ExpectedStatus: "waiting", Completion: *candidate})
	if errors.Is(err, store.ErrSessionRunTransitionConflict) {
		s.writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if transition.Changed {
		s.events.publish(transition.Event)
	}
	if transition.Run.Status == store.SessionRunStatusQueued {
		s.runs.wake(id)
	}
	writeJSON(w, http.StatusOK, apiSessionRun(transition.Run))
}
