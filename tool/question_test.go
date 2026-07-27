package tool

import (
	"context"
	"strings"
	"testing"
)

func TestQuestionToolExecutesAndFormatsAnswers(t *testing.T) {
	t.Parallel()
	var got Invocation
	question := NewQuestionTool(func(_ context.Context, inv Invocation, questions []Question) ([][]string, error) {
		got = inv
		if len(questions) != 1 || questions[0].Question != "Continue?" {
			t.Fatalf("questions = %#v", questions)
		}
		return [][]string{{"Yes"}}, nil
	})

	result, err := question.Execute(context.Background(), Invocation{
		CallID: "call_question",
		Input: map[string]any{"questions": []any{map[string]any{
			"question": "Continue?",
			"header":   "Next step",
			"options":  []any{map[string]any{"label": "Yes", "description": "Proceed"}},
		}}},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.CallID != "call_question" {
		t.Fatalf("call ID = %q", got.CallID)
	}
	if !strings.Contains(result.Text, `"Continue?"="Yes"`) {
		t.Fatalf("result text = %q", result.Text)
	}
}

func TestQuestionToolRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	question := NewQuestionTool(func(context.Context, Invocation, []Question) ([][]string, error) { return nil, nil })
	for name, input := range map[string]map[string]any{
		"missing questions": {},
		"empty questions":   {"questions": []any{}},
		"missing header": {"questions": []any{map[string]any{
			"question": "Continue?", "options": []any{map[string]any{"label": "Yes", "description": "Proceed"}},
		}}},
		"missing option description": {"questions": []any{map[string]any{
			"question": "Continue?", "header": "Next step", "options": []any{map[string]any{"label": "Yes"}},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := question.Execute(context.Background(), Invocation{Input: input}); err == nil {
				t.Fatal("Execute() error = nil")
			}
		})
	}
}
