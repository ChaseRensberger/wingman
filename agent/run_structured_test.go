package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
)

// TestRunStructuredReturnsTypedValue answers: does RunStructured return
// the parsed struct when the model emits valid JSON?
func TestRunStructuredReturnsTypedValue(t *testing.T) {
	type Contact struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	model := looptest.NewRecordingModel(looptest.Reply(`{"name":"Alice","email":"alice@example.com"}`))
	model.SetInfo(models.ModelInfo{
		Provider:     "looptest",
		ID:           "recording",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})

	s := session.New(session.WithModel(model))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, result, err := agent.RunStructured[Contact](ctx, s, "give me a contact")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", got.Name)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", got.Email)
	}
	if result == nil || result.StructuredOutput == nil {
		t.Fatal("expected StructuredOutput on result")
	}
}

// TestParseStructuredOnRunResult answers: does ParseStructured convert a
// Result with StructuredOutput into the target type?
func TestParseStructuredOnRunResult(t *testing.T) {
	type Contact struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	result := &session.Result{
		StructuredOutput: map[string]any{"name": "Bob", "email": "bob@example.com"},
	}
	got, err := agent.ParseStructured[Contact](result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Bob" {
		t.Errorf("Name = %q, want Bob", got.Name)
	}
}

// TestParseStructuredErrorsWhenNoStructuredOutput answers: does
// ParseStructured return an error when the result lacks StructuredOutput?
func TestParseStructuredErrorsWhenNoStructuredOutput(t *testing.T) {
	type Contact struct{}
	result := &session.Result{}
	_, err := agent.ParseStructured[Contact](result)
	if err == nil {
		t.Fatal("expected error when no structured output")
	}
}
