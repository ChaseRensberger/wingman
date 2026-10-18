package store

import (
	"errors"
	"strings"
	"time"
)

// CompletionOptions configures answer validation for a run.
type CompletionOptions struct {
	Remote           bool `json:"remote,omitempty"`
	MaxContinuations int  `json:"max_continuations,omitempty"`
}

// Completion records a candidate and its durable validation outcome.
type Completion struct {
	CandidateID string `json:"candidate_id"`
	Step        int    `json:"step"`
	Status      string `json:"status"`
	Feedback    string `json:"feedback,omitempty"`
	FeedbackID  string `json:"feedback_id,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CompletionUpdate compares the previous candidate status before saving a decision.
type CompletionUpdate struct {
	SessionID      string
	RunID          string
	ExpectedStatus string
	Completion     Completion
}

// ApplyCompletionUpdate applies the shared memory and SQLite transition rules.
func ApplyCompletionUpdate(current SessionRun, update CompletionUpdate) (SessionRun, bool, error) {
	if current.SessionID != update.SessionID || current.ID != update.RunID {
		return current, false, ErrSessionRunNotFound
	}
	c := update.Completion
	if c.CandidateID == "" || c.Step < 1 {
		return current, false, errors.New("completion candidate and step are required")
	}
	switch c.Status {
	case "validating", "waiting", "accept":
		if c.Feedback != "" || c.Error != "" {
			return current, false, errors.New("completion has unexpected feedback or error")
		}
	case "continue":
		if strings.TrimSpace(c.Feedback) == "" || c.Error != "" {
			return current, false, errors.New("completion continuation requires feedback")
		}
	case "error":
		if c.Error == "" || c.Feedback != "" {
			return current, false, errors.New("completion error requires a reason")
		}
	default:
		return current, false, errors.New("invalid completion status")
	}
	index := -1
	for i, existing := range current.Completions {
		if existing.CandidateID != c.CandidateID {
			continue
		}
		if existing.Step == c.Step && existing.Status == c.Status && existing.Feedback == c.Feedback && existing.Error == c.Error {
			return current, false, nil
		}
		index = i
	}
	if isSessionRunTerminal(current.Status) {
		return current, false, ErrSessionRunTransitionConflict
	}
	if index == -1 {
		if current.Status != SessionRunStatusRunning || update.ExpectedStatus != "" || c.Status != "validating" {
			return current, false, ErrSessionRunTransitionConflict
		}
		if len(current.Completions) > 0 {
			last := current.Completions[len(current.Completions)-1]
			if last.Status != "continue" || last.Step >= c.Step {
				return current, false, ErrSessionRunTransitionConflict
			}
		}
	} else {
		old := current.Completions[index]
		if index != len(current.Completions)-1 || old.Step != c.Step || old.Status != update.ExpectedStatus {
			return current, false, ErrSessionRunTransitionConflict
		}
		valid := old.Status == "validating" && current.Status == SessionRunStatusRunning && (c.Status == "accept" || c.Status == "continue" || c.Status == "error" || (c.Status == "waiting" && current.Completion.Remote))
		valid = valid || (old.Status == "waiting" && current.Status == SessionRunStatusAwaitingCompletion && (c.Status == "accept" || c.Status == "continue"))
		if !valid {
			return current, false, ErrSessionRunTransitionConflict
		}
	}
	c.FeedbackID = ""
	if c.Status == "continue" {
		c.FeedbackID = NewID(PrefixMessage)
	}
	current.Completions = append([]Completion(nil), current.Completions...)
	if index < 0 {
		current.Completions = append(current.Completions, c)
	} else {
		current.Completions[index] = c
	}
	if c.Status == "waiting" {
		current.Status = SessionRunStatusAwaitingCompletion
	} else if current.Status == SessionRunStatusAwaitingCompletion {
		current.Status = SessionRunStatusQueued
	}
	current.UpdatedAt = time.Now().UTC()
	return current, true, nil
}
