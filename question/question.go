// Package question defines requests for information and their correlated replies.
package question

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// ErrPending tells an executor to suspend until a question is resolved.
var ErrPending = errors.New("question pending")

// ErrNotFound identifies a question absent from the requested session.
var ErrNotFound = errors.New("question not found")

// ErrConflict identifies conflicting creation or resolution of a question.
var ErrConflict = errors.New("question conflict")

// ErrInvalid identifies an invalid question or answer.
var ErrInvalid = errors.New("invalid question")

// Choice is one suggested answer.
type Choice struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Prompt asks for one answer or a set of answers; custom text is always accepted.
type Prompt struct {
	Question string   `json:"question"`
	Header   string   `json:"header"`
	Options  []Choice `json:"options"`
	Multiple bool     `json:"multiple,omitempty"`
}

// Question records a session's request to its owning client.
// RunID and ToolUseID are optional: applications can ask independently of tools.
type Question struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	ClientID  string     `json:"client_id"`
	RunID     string     `json:"run_id,omitempty"`
	ToolUseID string     `json:"tool_use_id,omitempty"`
	Questions []Prompt   `json:"questions"`
	Status    string     `json:"status"`
	Answers   [][]string `json:"answers,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Reply answers or dismisses a question.
type Reply struct {
	Status  string     `json:"status"`
	Answers [][]string `json:"answers,omitempty"`
}

// Validate checks the question's prompts and origin.
func (q Question) Validate() error {
	if q.SessionID == "" || len(q.Questions) == 0 || (q.ToolUseID != "" && q.RunID == "") {
		return fmt.Errorf("%w: session and prompts are required; tool questions require a run", ErrInvalid)
	}
	for _, p := range q.Questions {
		if strings.TrimSpace(p.Question) == "" || strings.TrimSpace(p.Header) == "" {
			return fmt.Errorf("%w: prompt and header are required", ErrInvalid)
		}
		labels := map[string]bool{}
		for _, option := range p.Options {
			if strings.TrimSpace(option.Label) == "" || labels[option.Label] {
				return fmt.Errorf("%w: option labels must be nonempty and unique", ErrInvalid)
			}
			labels[option.Label] = true
		}
	}
	return nil
}

// SameRequest reports whether two creations describe the same request.
func SameRequest(a, b Question) bool {
	return a.SessionID == b.SessionID && a.RunID == b.RunID && a.ToolUseID == b.ToolUseID && reflect.DeepEqual(a.Questions, b.Questions)
}

// Resolve validates a reply and applies it once. Identical retries are harmless.
func Resolve(q Question, reply Reply) (Question, bool, error) {
	if reply.Status != "answered" && reply.Status != "dismissed" && reply.Status != "cancelled" {
		return q, false, fmt.Errorf("%w: unsupported reply status", ErrInvalid)
	}
	if reply.Status == "answered" {
		if len(reply.Answers) != len(q.Questions) {
			return q, false, fmt.Errorf("%w: supply an answer for each prompt", ErrInvalid)
		}
		for i, answers := range reply.Answers {
			if len(answers) == 0 || (!q.Questions[i].Multiple && len(answers) != 1) {
				return q, false, fmt.Errorf("%w: invalid selection count", ErrInvalid)
			}
			seen := map[string]bool{}
			for _, answer := range answers {
				if strings.TrimSpace(answer) == "" || seen[answer] {
					return q, false, fmt.Errorf("%w: answers must be nonempty and unique", ErrInvalid)
				}
				seen[answer] = true
			}
		}
	} else if len(reply.Answers) != 0 {
		return q, false, fmt.Errorf("%w: dismissal or cancellation cannot contain answers", ErrInvalid)
	}
	if reply.Status != "answered" {
		reply.Answers = nil
	}
	if q.Status != "pending" {
		if q.Status == reply.Status && reflect.DeepEqual(q.Answers, reply.Answers) {
			return q, false, nil
		}
		return q, false, ErrConflict
	}
	q.Status, q.Answers, q.UpdatedAt = reply.Status, reply.Answers, time.Now().UTC()
	return q, true, nil
}
