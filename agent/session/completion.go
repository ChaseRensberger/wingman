package session

import (
	"context"
	"errors"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

// WithBeforeComplete installs a validator after plugin validators.
func WithBeforeComplete(hook run.BeforeCompleteHook) Option {
	return func(s *Session) { s.beforeComplete = hook }
}

// WithCompletionOptions configures remote validation and the continuation limit.
// Stored runs use the options saved at admission instead.
func WithCompletionOptions(options store.CompletionOptions) Option {
	return func(s *Session) { s.completion = options }
}

// WithCompletionEventSink observes completion events after their transaction commits.
func WithCompletionEventSink(sink func(store.SessionEvent)) Option {
	return func(s *Session) { s.completionEventSink = sink }
}

type completionRecorder struct {
	session  *Session
	messages *sessionMessagePersistence
	limit    int
}

func (p *completionRecorder) Decide(ctx context.Context, info run.CompletionInfo, hook run.BeforeCompleteHook) (run.CompletionDecision, *models.Message, error) {
	s := p.session
	admitted, err := s.store.GetSessionRun(ctx, s.id, s.runID)
	if err != nil {
		return run.CompletionDecision{}, nil, err
	}
	c := store.Completion{CandidateID: info.Candidate.ID, Step: info.Step, Status: "validating"}
	found := false
	for _, existing := range admitted.Completions {
		if existing.CandidateID == c.CandidateID {
			c, found = existing, true
			break
		}
	}
	save := func(expected string) error {
		transition, err := s.store.UpdateSessionRunCompletion(ctx, store.CompletionUpdate{SessionID: s.id, RunID: s.runID, ExpectedStatus: expected, Completion: c})
		if err != nil {
			return err
		}
		c = transition.Run.Completions[len(transition.Run.Completions)-1]
		if transition.Changed && s.completionEventSink != nil {
			s.completionEventSink(transition.Event)
		}
		return nil
	}
	if !found {
		if err := save(""); err != nil {
			return run.CompletionDecision{}, nil, err
		}
	}
	if c.Status == "validating" {
		d := run.CompletionDecision{Action: "accept"}
		if hook != nil {
			d, err = hook(ctx, info)
		}
		if ctx.Err() != nil {
			return d, nil, ctx.Err()
		}
		if err == nil {
			err = d.Validate()
		}
		if err != nil {
			c.Status, c.Error = "error", err.Error()
		} else {
			c.Status, c.Feedback = d.Action, d.Feedback
			if d.Action == "accept" && admitted.Completion.Remote {
				c.Status = "waiting"
			}
		}
		if err := save("validating"); err != nil {
			return d, nil, err
		}
	}
	if c.Status == "waiting" {
		return run.CompletionDecision{}, nil, run.ErrCompletionPending
	}
	if c.Status == "error" {
		if err != nil {
			return run.CompletionDecision{}, nil, err
		}
		return run.CompletionDecision{}, nil, errors.New(c.Error)
	}
	d := run.CompletionDecision{Action: c.Status, Feedback: c.Feedback}
	if d.Action != "continue" {
		return d, nil, nil
	}
	limit := p.limit
	if limit == 0 {
		limit = 3
	}
	if info.Continuations >= limit {
		return d, nil, run.ErrCompletionLimit
	}
	feedback, err := p.messages.saveCompletionFeedback(ctx, c)
	return d, feedback, err
}

func (p *sessionMessagePersistence) saveCompletionFeedback(ctx context.Context, c store.Completion) (*models.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	existing, err := p.session.store.QueryMessages(ctx, p.session.id, store.MessageQuery{MessageID: c.FeedbackID})
	if err != nil {
		return nil, err
	}
	var message models.Message
	if len(existing) > 0 {
		message, err = storedMessageToModel(existing[0], p.session.partDecoders)
	} else {
		message = models.Message{ID: c.FeedbackID, Role: models.RoleUser, Content: models.Content{models.TextPart{Text: c.Feedback}}, Metadata: models.Meta{"source": "completion_validator", "candidate_id": c.CandidateID}}
		message, err = p.session.persistMessage(ctx, message, p.nextIdx)
		if err == nil {
			p.nextIdx++
		}
	}
	if err != nil {
		return nil, err
	}
	if p.feedbackIDs == nil {
		p.feedbackIDs = map[string]bool{}
	}
	p.feedbackIDs[message.ID] = true
	return &message, nil
}

func (p *sessionMessagePersistence) feedbackSaved(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.feedbackIDs[id]
}
