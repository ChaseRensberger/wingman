package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/question"
)

// QuestionHandler creates or retrieves a question for a tool invocation.
type QuestionHandler func(context.Context, Invocation, []question.Prompt) (question.Question, error)

// NewQuestionTool exposes structured questions through the supplied runtime.
func NewQuestionTool(ask QuestionHandler) Tool {
	const description = "Ask questions when you need information or a decision to continue. Supply suggested choices; custom text is always available. Use multiple to allow several answers."
	return NewFuncTool("question", description, Definition{
		Name: "question", Description: description, ReplaySafe: true,
		Permission: &PermissionTarget{Action: "question"},
		RawInputSchema: map[string]any{
			"type": "object", "required": []string{"questions"},
			"properties": map[string]any{"questions": map[string]any{
				"type": "array", "minItems": 1,
				"items": map[string]any{
					"type": "object", "required": []string{"question", "header", "options"},
					"properties": map[string]any{
						"question": map[string]any{"type": "string", "minLength": 1},
						"header":   map[string]any{"type": "string", "minLength": 1},
						"multiple": map[string]any{"type": "boolean"},
						"options": map[string]any{"type": "array", "items": map[string]any{
							"type": "object", "required": []string{"label", "description"},
							"properties": map[string]any{"label": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}},
						}},
					},
				},
			}},
		},
	}, func(ctx context.Context, inv Invocation) (Result, error) {
		if ask == nil {
			return Result{}, fmt.Errorf("questions are unavailable in this execution")
		}
		data, err := json.Marshal(inv.Input)
		if err != nil {
			return Result{}, err
		}
		var input struct {
			Questions []question.Prompt `json:"questions"`
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return Result{}, err
		}
		q, err := ask(ctx, inv, input.Questions)
		if err != nil {
			return Result{}, err
		}
		if q.Status == "pending" {
			return Result{}, question.ErrPending
		}
		if q.Status != "answered" {
			return Result{Text: "The question was " + q.Status + ". No answer was supplied.", Structured: q}, nil
		}
		var lines []string
		for i, prompt := range q.Questions {
			lines = append(lines, fmt.Sprintf("%s: %s", prompt.Question, strings.Join(q.Answers[i], ", ")))
		}
		return Result{Text: strings.Join(lines, "\n"), Structured: q}, nil
	})
}
