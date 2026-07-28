package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
	"github.com/go-chi/chi/v5"
)

var errQuestionDismissed = errors.New("The user dismissed this question")

type pendingQuestion struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	CallID    string          `json:"call_id"`
	Questions []tool.Question `json:"questions"`
	result    chan questionResult
}
type questionResult struct {
	answers [][]string
	err     error
}
type questionBroker struct {
	mu      sync.Mutex
	pending map[string]*pendingQuestion
}

func newQuestionBroker() *questionBroker {
	return &questionBroker{pending: map[string]*pendingQuestion{}}
}

func (b *questionBroker) ask(ctx context.Context, sessionID, callID string, questions []tool.Question, publish func(string, any)) ([][]string, error) {
	request := &pendingQuestion{ID: store.NewID("que_"), SessionID: sessionID, CallID: callID, Questions: questions, result: make(chan questionResult, 1)}
	b.mu.Lock()
	b.pending[request.ID] = request
	b.mu.Unlock()
	publish("session.question.asked", request)
	defer func() { b.mu.Lock(); delete(b.pending, request.ID); b.mu.Unlock() }()
	select {
	case result := <-request.result:
		return result.answers, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (b *questionBroker) list(sessionID string) []pendingQuestion {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []pendingQuestion{}
	for _, request := range b.pending {
		if request.SessionID == sessionID {
			out = append(out, *request)
		}
	}
	return out
}
func (b *questionBroker) answer(sessionID, id string, answers [][]string, dismiss bool) error {
	b.mu.Lock()
	request := b.pending[id]
	if request == nil || request.SessionID != sessionID {
		b.mu.Unlock()
		return store.ErrSessionNotFound
	}
	delete(b.pending, id)
	b.mu.Unlock()
	result := questionResult{answers: answers}
	if dismiss {
		result.err = errQuestionDismissed
	}
	request.result <- result
	return nil
}

func (s *Server) handleListSessionQuestions(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.questions.list(sessionID))
}

func (s *Server) handleReplySessionQuestion(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	var body struct {
		Answers [][]string `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.questions.answer(sessionID, chi.URLParam(r, "questionID"), body.Answers, false); err != nil {
		writeError(w, http.StatusNotFound, "question not found")
		return
	}
	s.persistRunEvent(context.Background(), sessionID, "session.question.replied", map[string]any{"question_id": chi.URLParam(r, "questionID"), "answers": body.Answers})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDismissSessionQuestion(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	questionID := chi.URLParam(r, "questionID")
	if err := s.questions.answer(sessionID, questionID, nil, true); err != nil {
		writeError(w, http.StatusNotFound, "question not found")
		return
	}
	s.persistRunEvent(context.Background(), sessionID, "session.question.dismissed", map[string]any{"question_id": questionID})
	w.WriteHeader(http.StatusNoContent)
}
