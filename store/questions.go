package store

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/chaserensberger/wingman/question"
)

// QuestionTransition contains a committed question and its notification events.
type QuestionTransition struct {
	Question question.Question
	Events   []SessionEvent
	Changed  bool
}

// NewSessionQuestionEvent records a complete question snapshot.
func NewSessionQuestionEvent(q question.Question) (AggregateEvent, error) {
	data, err := json.Marshal(q)
	if err != nil {
		return AggregateEvent{}, err
	}
	return AggregateEvent{ID: NewID(PrefixEvent), Aggregate: AggregateRef{Type: AggregateSession, ID: q.SessionID}, Type: "session.question." + q.Status, SchemaVersion: 1, Time: q.UpdatedAt, Data: data, ClientID: q.ClientID, RunID: q.RunID}, nil
}

// ProjectSessionQuestion decodes a question snapshot from aggregate history.
func ProjectSessionQuestion(event AggregateEvent) (question.Question, error) {
	var q question.Question
	if err := json.Unmarshal(event.Data, &q); err != nil {
		return q, err
	}
	if event.SchemaVersion != 1 || q.ID == "" || q.SessionID != event.Aggregate.ID || event.Type != "session.question."+q.Status {
		return q, fmt.Errorf("invalid question event")
	}
	return q, q.Validate()
}

// ProjectSessionQuestions rebuilds question state from aggregate history.
func ProjectSessionQuestions(events []AggregateEvent) ([]question.Question, error) {
	if _, err := ProjectSession(events); err != nil {
		return nil, err
	}
	byID := map[string]question.Question{}
	for _, event := range events {
		switch event.Type {
		case "session.question.pending", "session.question.answered", "session.question.dismissed", "session.question.cancelled":
			q, err := ProjectSessionQuestion(event)
			if err != nil {
				return nil, err
			}
			if old, ok := byID[q.ID]; ok {
				if !question.SameRequest(old, q) || old.Status != "pending" || q.Status == "pending" {
					return nil, question.ErrConflict
				}
			} else if q.Status != "pending" {
				return nil, question.ErrConflict
			}
			byID[q.ID] = q
		}
	}
	out := make([]question.Question, 0, len(byID))
	for _, q := range byID {
		out = append(out, q)
	}
	SortQuestions(out)
	return out, nil
}

// SortQuestions orders questions by creation time and identity.
func SortQuestions(questions []question.Question) {
	slices.SortFunc(questions, func(a, b question.Question) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
}

// NewSessionInputEvent records a run's wait or resumption at a question boundary.
func NewSessionInputEvent(run SessionRun) (AggregateEvent, error) {
	event, err := NewSessionCompletionEvent(run)
	event.Type = "session.run.input"
	return event, err
}
