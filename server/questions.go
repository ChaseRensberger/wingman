package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleCreateQuestion(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	var input api.CreateQuestionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	transition, err := s.store.CreateQuestion(r.Context(), question.Question{ID: input.ID, SessionID: id, Questions: input.Questions})
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	for _, event := range transition.Events {
		s.events.publish(event)
	}
	writeJSON(w, http.StatusOK, transition.Question)
}

func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	questions, err := s.store.ListQuestions(r.Context(), id)
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

func (s *Server) handleGetQuestion(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	q, err := s.store.GetQuestion(r.Context(), id, chi.URLParam(r, "questionID"))
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleReplyQuestion(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, id); !ok {
		return
	}
	var input api.QuestionReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Status != "answered" && input.Status != "dismissed" {
		s.writeError(w, http.StatusBadRequest, "status must be answered or dismissed")
		return
	}
	transition, err := s.store.ResolveQuestion(r.Context(), id, chi.URLParam(r, "questionID"), question.Reply{Status: input.Status, Answers: input.Answers})
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	for _, event := range transition.Events {
		s.events.publish(event)
	}
	if transition.Question.RunID != "" {
		s.runs.wake(id)
	}
	writeJSON(w, http.StatusOK, transition.Question)
}

func (s *Server) writeQuestionError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, question.ErrNotFound), errors.Is(err, store.ErrSessionNotFound):
		status = http.StatusNotFound
	case errors.Is(err, question.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, question.ErrInvalid):
		status = http.StatusBadRequest
	}
	s.writeError(w, status, err.Error())
}
