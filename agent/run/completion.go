package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// ErrCompletionPending suspends a run until its remote validator replies.
var ErrCompletionPending = errors.New("completion validation pending")

// ErrCompletionLimit means a validator requested more iterations than allowed.
var ErrCompletionLimit = errors.New("completion validation continuation limit reached")

// CompletionDecision accepts a candidate or requests another iteration with feedback.
type CompletionDecision struct {
	Action   string `json:"action" enum:"accept,continue"`
	Feedback string `json:"feedback,omitempty"`
}

// Validate checks the decision's action and feedback.
func (d CompletionDecision) Validate() error {
	if (d.Action == "accept" && d.Feedback == "") || (d.Action == "continue" && strings.TrimSpace(d.Feedback) != "") {
		return nil
	}
	return errors.New("completion decision requires accept, or continue with nonempty feedback")
}

// CompletionInfo identifies the candidate and the run's completed continuations.
// Candidate and Messages are read-only snapshots.
type CompletionInfo struct {
	SessionID     string
	RunID         string
	Step          int
	Continuations int
	Candidate     models.Message
	Messages      []models.Message
	Usage         models.Usage
}

// BeforeCompleteHook validates a normal answer before the run finishes.
// Uncommitted callbacks can repeat after restart; use Candidate.ID for idempotency.
type BeforeCompleteHook func(context.Context, CompletionInfo) (CompletionDecision, error)

// CompletionLifecycle persists validation and returns any durable feedback message.
type CompletionLifecycle interface {
	Decide(context.Context, CompletionInfo, BeforeCompleteHook) (CompletionDecision, *models.Message, error)
}

// ComposeBeforeComplete runs validators in order until one continues or fails.
func ComposeBeforeComplete(hooks ...BeforeCompleteHook) BeforeCompleteHook {
	return func(ctx context.Context, info CompletionInfo) (CompletionDecision, error) {
		for _, hook := range hooks {
			if hook == nil {
				continue
			}
			d, err := hook(ctx, info)
			if err != nil {
				return d, err
			}
			if err := d.Validate(); err != nil {
				return d, err
			}
			if d.Action == "continue" {
				return d, nil
			}
		}
		return CompletionDecision{Action: "accept"}, nil
	}
}

func (r *runner) complete(ctx context.Context, turn Turn) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	output, err := r.parseStructuredOutput(turn)
	if err != nil {
		return false, err
	}
	info := CompletionInfo{SessionID: r.cfg.SessionID, RunID: r.cfg.RunID, Step: turn.Step, Continuations: r.continuations, Candidate: turn.Assistant, Messages: append([]models.Message(nil), r.messages...), Usage: r.usage}
	decision := CompletionDecision{Action: "accept"}
	var feedback *models.Message
	if r.cfg.CompletionLifecycle != nil {
		decision, feedback, err = r.cfg.CompletionLifecycle.Decide(ctx, info, r.cfg.Hooks.BeforeComplete)
	} else if r.cfg.Hooks.BeforeComplete != nil {
		decision, err = r.cfg.Hooks.BeforeComplete(ctx, info)
	}
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := decision.Validate(); err != nil {
		return false, err
	}
	if decision.Action == "accept" {
		if output != nil {
			r.structuredOutput = output.Parsed
			r.emit(*output)
		}
		return true, nil
	}
	limit := r.cfg.MaxCompletionContinuations
	if limit == 0 {
		limit = 3
	}
	if r.continuations >= limit {
		return false, ErrCompletionLimit
	}
	if r.cfg.MaxSteps > 0 && turn.Step >= r.cfg.MaxSteps {
		return false, fmt.Errorf("%w: step limit reached", ErrCompletionLimit)
	}
	if feedback == nil {
		feedback = &models.Message{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: decision.Feedback}}, Metadata: map[string]any{"source": "completion_validator", "candidate_id": turn.Assistant.ID}}
	}
	found := false
	for _, message := range r.messages {
		if feedback.ID != "" && message.ID == feedback.ID {
			found = true
			break
		}
	}
	if !found {
		r.messages = append(r.messages, *feedback)
		r.emitHistoryEvent(MessageEvent{Step: turn.Step, Message: *feedback})
	}
	r.continuations++
	return false, nil
}

func (r *runner) completionResult(ctx context.Context, step int, err error) (*Result, error) {
	reason := StopReasonError
	if errors.Is(err, ErrCompletionPending) {
		reason = StopReasonAwaitingCompletion
	} else {
		r.emitError(err)
		if ctx.Err() != nil {
			reason = StopReasonAborted
		}
	}
	return r.finalize(step, reason), err
}
