package run

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/tool"
)

func TestQuestionsSuspendSuccessiveRecoveredToolCalls(t *testing.T) {
	const explanation = "Choose a color before I continue."
	providerCalls, ends := 0, 0
	client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		providerCalls++
		content := models.Content{models.TextPart{Text: "done"}}
		if providerCalls == 1 {
			input := map[string]any{"questions": []any{map[string]any{"question": "Which color?", "header": "Color", "options": []any{}}}}
			content = models.Content{
				models.TextPart{Text: explanation},
				models.ToolCallPart{ID: "part_first", CallID: "first", Name: "question", Input: input},
				models.ToolCallPart{ID: "part_second", CallID: "second", Name: "question", Input: input},
			}
		}
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: content}, nil)
		return stream, nil
	}}
	answered := map[string]bool{}
	invocations := map[string]int{}
	ask := tool.NewQuestionTool(func(_ context.Context, inv tool.Invocation, prompts []question.Prompt) (question.Question, error) {
		invocations[inv.CallID]++
		q := question.Question{Questions: prompts, Status: "pending"}
		if answered[inv.CallID] {
			q.Status, q.Answers = "answered", [][]string{{"Blue"}}
		}
		return q, nil
	})
	var snapshots []models.Message
	cfg := Config{Client: client, Model: testModel, Tools: []tool.Tool{ask}, ToolExecution: ToolExecutionSequential,
		Sink: SinkFunc(func(event Event) {
			if message, ok := event.(MessageEvent); ok {
				snapshots = append(snapshots, message.Message)
			}
		}),
		Hooks: Hooks{AfterRun: func(context.Context, AfterRunInfo) error { ends++; return nil }},
	}
	for i, callID := range []string{"first", "second"} {
		result, err := Run(t.Context(), cfg)
		if !errors.Is(err, question.ErrPending) || result.StopReason != StopReasonAwaitingInput || len(result.Turns) != 1 {
			t.Fatalf("suspension %d: stop=%s turns=%d err=%v", i, result.StopReason, len(result.Turns), err)
		}
		if providerCalls != 1 || ends != 0 || len(snapshots) != i+1 {
			t.Fatalf("suspension %d: provider=%d ends=%d snapshots=%d", i, providerCalls, ends, len(snapshots))
		}
		message := snapshots[i]
		if message.State != models.MessageStateInProgress || message.Content[0].(models.TextPart).Text != explanation {
			t.Fatalf("missing suspended explanation: %#v", message)
		}
		if len(result.Turns[0].Results) != i {
			t.Fatalf("suspension %d lost completed results: %#v", i, result.Turns[0].Results)
		}
		answered[callID] = true
		recovery := &Recovery{Step: result.Steps, Turn: &result.Turns[0], Tools: map[string]RecoveredTool{}}
		for _, saved := range result.Turns[0].Results {
			recovery.Tools["part_"+saved.CallID] = RecoveredTool{Result: &saved}
		}
		cfg.Recovery = recovery
	}
	result, err := Run(t.Context(), cfg)
	if err != nil || result.StopReason != StopReasonEndTurn || providerCalls != 2 || ends != 1 {
		t.Fatalf("completion: result=%#v provider=%d ends=%d err=%v", result, providerCalls, ends, err)
	}
	if invocations["first"] != 2 || invocations["second"] != 2 {
		t.Fatalf("completed question was repeated: %v", invocations)
	}
}
