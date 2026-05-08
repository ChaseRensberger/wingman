package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chaserensberger/wingman/agent/session"
)

// RunStructured runs the session with an output schema bound to type T.
// The session's current OutputSchema is overridden by SchemaFor[T]()
// for this run. Returns the typed value, the full Result, and any error.
// Parse and validation failures return a non-nil error and a zero T.
func RunStructured[T any](ctx context.Context, s *session.Session, message string) (T, *session.Result, error) {
	var zero T
	schema := SchemaFor[T]()
	old := s.OutputSchema()
	s.SetOutputSchema(schema)
	defer s.SetOutputSchema(old)
	result, err := s.Run(ctx, message)
	if err != nil {
		return zero, result, err
	}
	parsed, err := ParseStructured[T](result)
	if err != nil {
		return zero, result, err
	}
	return parsed, result, nil
}

// ParseStructured unmarshals a session.Result's structured output into T.
// Returns an error if the result had no structured output or the JSON
// doesn't match T.
func ParseStructured[T any](r *session.Result) (T, error) {
	var zero T
	if r == nil || r.StructuredOutput == nil {
		return zero, fmt.Errorf("agent: run result has no structured output")
	}
	b, err := json.Marshal(r.StructuredOutput)
	if err != nil {
		return zero, fmt.Errorf("agent: re-marshal structured output: %w", err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, fmt.Errorf("agent: unmarshal structured output into %T: %w", zero, err)
	}
	return out, nil
}
