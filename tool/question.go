package tool

import (
	"context"
	"fmt"
)

// Question is one user decision requested by the agent.
type Question struct {
	Question string           `json:"question"`
	Header   string           `json:"header"`
	Options  []QuestionOption `json:"options"`
	Multiple bool             `json:"multiple,omitempty"`
	Custom   bool             `json:"custom,omitempty"`
}

// QuestionOption is one selectable answer.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// QuestionTool waits for a user response supplied by its owning session host.
type QuestionTool struct {
	ask func(context.Context, Invocation, []Question) ([][]string, error)
}

// NewQuestionTool creates a question tool bound to one session host.
func NewQuestionTool(ask func(context.Context, Invocation, []Question) ([][]string, error)) *QuestionTool {
	return &QuestionTool{ask: ask}
}

func (t *QuestionTool) Name() string { return "question" }

func (t *QuestionTool) Description() string {
	return "Ask the user one or more questions during execution. Use concise labeled options; set multiple to allow more than one answer."
}

func (t *QuestionTool) Definition() Definition {
	return Definition{Name: t.Name(), Description: t.Description(), RawInputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{"type": "string"}, "header": map[string]any{"type": "string"}, "multiple": map[string]any{"type": "boolean"}, "custom": map[string]any{"type": "boolean"},
						"options": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}}, "required": []string{"label", "description"}}},
					},
					"required": []string{"question", "header", "options"},
				},
			},
		},
		"required": []string{"questions"},
	}}
}

func (t *QuestionTool) Sequential() bool { return true }

func (t *QuestionTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	questions, err := parseQuestions(inv.Input)
	if err != nil {
		return Result{}, err
	}
	if t.ask == nil {
		return Result{}, fmt.Errorf("question tool is unavailable")
	}
	answers, err := t.ask(ctx, inv, questions)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: formatQuestionAnswers(questions, answers), Metadata: map[string]any{"questions": questions, "answers": answers}}, nil
}

func parseQuestions(input map[string]any) ([]Question, error) {
	raw, ok := input["questions"].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("questions is required")
	}
	questions := make([]Question, 0, len(raw))
	for _, value := range raw {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("question must be an object")
		}
		q := Question{Question: stringInput(item, "question"), Header: stringInput(item, "header"), Multiple: boolInput(item, "multiple"), Custom: true}
		if q.Question == "" || q.Header == "" {
			return nil, fmt.Errorf("question and header are required")
		}
		if custom, ok := item["custom"].(bool); ok {
			q.Custom = custom
		}
		rawOptions, ok := item["options"].([]any)
		if !ok || len(rawOptions) == 0 {
			return nil, fmt.Errorf("question options are required")
		}
		for _, rawOption := range rawOptions {
			option, ok := rawOption.(map[string]any)
			if !ok || stringInput(option, "label") == "" || stringInput(option, "description") == "" {
				return nil, fmt.Errorf("option label and description are required")
			}
			q.Options = append(q.Options, QuestionOption{Label: stringInput(option, "label"), Description: stringInput(option, "description")})
		}
		questions = append(questions, q)
	}
	return questions, nil
}

func stringInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}
func boolInput(input map[string]any, key string) bool { value, _ := input[key].(bool); return value }

func formatQuestionAnswers(questions []Question, answers [][]string) string {
	text := "User has answered your questions:"
	for i, question := range questions {
		answer := "Unanswered"
		if i < len(answers) && len(answers[i]) > 0 {
			answer = ""
			for j, value := range answers[i] {
				if j > 0 {
					answer += ", "
				}
				answer += value
			}
		}
		text += fmt.Sprintf(" %q=%q", question.Question, answer)
	}
	return text + ". You can now continue with the user's answers in mind."
}
