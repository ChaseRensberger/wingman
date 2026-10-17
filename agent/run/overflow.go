package run

import (
	"context"

	"github.com/chaserensberger/wingman/models"
)

func (r *runner) scheduleOverflowRecovery(ctx context.Context, turn *Turn, message models.Message, failure error) {
	if ctx.Err() == nil && !r.overflowRecovered && r.cfg.Hooks.TransformHistory != nil &&
		len(message.Content) == 0 && contextOverflowFailure(failure) &&
		(r.cfg.MaxSteps == 0 || turn.Step < r.cfg.MaxSteps) {
		turn.Trace.Retry = &models.CallRetry{Decision: "scheduled", Reason: "context_overflow"}
	}
}

// Joined persistence or lifecycle failures must never be mistaken for a retryable rejection.
func contextOverflowFailure(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !contextOverflowFailure(child) {
				return false
			}
		}
		return true
	}
	if provider, ok := err.(*models.ProviderError); ok {
		return provider.Classification == "context-overflow"
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return contextOverflowFailure(wrapped.Unwrap())
	}
	return false
}
