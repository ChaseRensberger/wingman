package memory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
)

func cloneQuestion(q question.Question) question.Question {
	data, _ := json.Marshal(q)
	var copy question.Question
	_ = json.Unmarshal(data, &copy)
	return copy
}

// CreateQuestion saves a request once, including when a tool is resumed.
func (s *Store) CreateQuestion(ctx context.Context, q question.Question) (store.QuestionTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return store.QuestionTransition{}, err
	}
	if err := q.Validate(); err != nil {
		return store.QuestionTransition{}, err
	}
	session, ok := s.sessions[q.SessionID]
	if !ok {
		return store.QuestionTransition{}, store.ErrSessionNotFound
	}
	q.ClientID = session.ClientID
	if q.ID == "" {
		q.ID = store.NewID(store.PrefixQuestion)
	}
	for _, old := range s.questions {
		if old.ID == q.ID || (q.ToolUseID != "" && old.ToolUseID == q.ToolUseID) {
			if !question.SameRequest(old, q) {
				return store.QuestionTransition{}, question.ErrConflict
			}
			return store.QuestionTransition{Question: cloneQuestion(old)}, nil
		}
	}
	if q.RunID != "" {
		run, ok := s.runs[q.RunID]
		if !ok || run.SessionID != q.SessionID {
			return store.QuestionTransition{}, store.ErrSessionRunNotFound
		}
		if run.Status != store.SessionRunStatusRunning {
			return store.QuestionTransition{}, question.ErrConflict
		}
	}
	if q.ToolUseID != "" {
		use, ok := s.toolUses[q.ToolUseID]
		if !ok || use.SessionID != q.SessionID || use.RunID != q.RunID {
			return store.QuestionTransition{}, question.ErrConflict
		}
	}
	q.Status, q.Answers = "pending", nil
	q.CreatedAt = time.Now().UTC()
	q.UpdatedAt = q.CreatedAt
	event, err := s.saveQuestionLocked(q)
	if err != nil {
		return store.QuestionTransition{}, err
	}
	return store.QuestionTransition{Question: cloneQuestion(q), Events: []store.SessionEvent{event}, Changed: true}, nil
}

// GetQuestion returns a question owned by the session.
func (s *Store) GetQuestion(ctx context.Context, sessionID, id string) (question.Question, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return question.Question{}, err
	}
	q, ok := s.questions[id]
	if !ok || q.SessionID != sessionID {
		return question.Question{}, question.ErrNotFound
	}
	return cloneQuestion(q), nil
}

// ListQuestions returns all questions in creation order.
func (s *Store) ListQuestions(ctx context.Context, sessionID string) ([]question.Question, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	out := []question.Question{}
	for _, q := range s.questions {
		if q.SessionID == sessionID {
			out = append(out, cloneQuestion(q))
		}
	}
	store.SortQuestions(out)
	return out, nil
}

// ResolveQuestion commits a reply and wakes a run when its questions are resolved.
func (s *Store) ResolveQuestion(ctx context.Context, sessionID, id string, reply question.Reply) (store.QuestionTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return store.QuestionTransition{}, err
	}
	q, ok := s.questions[id]
	if !ok || q.SessionID != sessionID {
		return store.QuestionTransition{}, question.ErrNotFound
	}
	q, changed, err := question.Resolve(cloneQuestion(q), reply)
	if err != nil || !changed {
		return store.QuestionTransition{Question: q}, err
	}
	event, err := s.saveQuestionLocked(q)
	if err != nil {
		return store.QuestionTransition{}, err
	}
	events := []store.SessionEvent{event}
	if run := s.runs[q.RunID]; run != nil && run.Status == store.SessionRunStatusAwaitingInput && !s.pendingQuestionsLocked(run.ID) {
		candidate := copySessionRun(run)
		candidate.Status = store.SessionRunStatusQueued
		event, err := s.saveInputRunLocked(candidate)
		if err != nil {
			return store.QuestionTransition{}, err
		}
		events = append(events, event)
	}
	return store.QuestionTransition{Question: cloneQuestion(q), Events: events, Changed: true}, nil
}

// SuspendSessionRunForQuestions releases a run after its tool batch has stopped.
func (s *Store) SuspendSessionRunForQuestions(ctx context.Context, sessionID, runID string) (store.SessionRunTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return store.SessionRunTransition{}, err
	}
	current := s.runs[runID]
	if current == nil || current.SessionID != sessionID {
		return store.SessionRunTransition{}, store.ErrSessionRunNotFound
	}
	if current.Status != store.SessionRunStatusRunning {
		return store.SessionRunTransition{}, store.ErrSessionRunTransitionConflict
	}
	run := copySessionRun(current)
	run.Status = store.SessionRunStatusAwaitingInput
	if !s.pendingQuestionsLocked(runID) {
		run.Status = store.SessionRunStatusQueued
	}
	event, err := s.saveInputRunLocked(run)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	return store.SessionRunTransition{Run: copySessionRun(s.runs[runID]), Event: event, Changed: true}, nil
}

func (s *Store) pendingQuestionsLocked(runID string) bool {
	for _, q := range s.questions {
		if q.RunID == runID && q.Status == "pending" {
			return true
		}
	}
	return false
}

func (s *Store) saveInputRunLocked(run store.SessionRun) (store.SessionEvent, error) {
	run.UpdatedAt = time.Now().UTC()
	aggregate, err := store.NewSessionInputEvent(run)
	if err != nil {
		return store.SessionEvent{}, err
	}
	if err := s.appendQuestionAggregateLocked(aggregate); err != nil {
		return store.SessionEvent{}, err
	}
	event, err := s.appendRunEventLocked(&run, "session.run.input", nil, run.UpdatedAt)
	if err != nil {
		return store.SessionEvent{}, err
	}
	s.runs[run.ID] = &run
	return event, nil
}

func (s *Store) appendQuestionAggregateLocked(event store.AggregateEvent) error {
	session := s.sessions[event.Aggregate.ID]
	event.Version = session.AggregateVersion + 1
	projected, err := store.ApplySessionEvent(session, event)
	if err != nil {
		return err
	}
	s.appendPermissionAggregateCommitLocked(event, projected)
	return nil
}

func (s *Store) saveQuestionLocked(q question.Question) (store.SessionEvent, error) {
	aggregate, err := store.NewSessionQuestionEvent(q)
	if err != nil {
		return store.SessionEvent{}, err
	}
	if err := s.appendQuestionAggregateLocked(aggregate); err != nil {
		return store.SessionEvent{}, err
	}
	var seq int64
	for _, event := range s.events {
		if event.SessionID == q.SessionID && event.Seq > seq {
			seq = event.Seq
		}
	}
	event := store.SessionEvent{ID: store.NewID(store.PrefixEvent), SessionID: q.SessionID, Seq: seq + 1, SchemaVersion: 1, Type: aggregate.Type, Time: q.UpdatedAt, DataJSON: aggregate.Data, Data: aggregate.Data}
	s.events[event.ID] = &event
	s.questions[q.ID] = cloneQuestion(q)
	return event, nil
}
