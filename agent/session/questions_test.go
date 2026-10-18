package session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

type questionClient struct {
	requestCaptureClient
	calls         int
	requests      []models.Request
	questionFirst bool
}

func (c *questionClient) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.calls++
	c.requests = append(c.requests, req)
	message := models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}
	if c.calls == 1 {
		message.Content = models.Content{
			models.ToolCallPart{CallID: "effect", Name: "effect", Input: map[string]any{}},
			models.ToolCallPart{CallID: "question", Name: "question", Input: map[string]any{"questions": []any{map[string]any{"question": "Which?", "header": "Choice", "options": []any{}}}}},
		}
		if c.questionFirst {
			message.Content[0], message.Content[1] = message.Content[1], message.Content[0]
		}
	}
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)
	stream.Close(&message, nil)
	return stream, nil
}

func TestQuestionSuspensionResumesOriginalToolBatch(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		for _, sequential := range []bool{false, true} {
			t.Run(backend, func(t *testing.T) {
				data := historyStore(t, backend)
				admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Message: "go"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
					t.Fatal(err)
				}
				client := &questionClient{questionFirst: sequential}
				effects := 0
				effect := tool.NewFuncTool("effect", "effect", tool.Definition{Name: "effect", Sequential: sequential, InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
					effects++
					return tool.Result{Text: "effect complete"}, nil
				})
				base := append(completionOptions(data, admitted.Run.ID, client), WithTools(effect, QuestionTool(data, nil)))
				s := New(base...)
				result, err := s.Run(t.Context(), "go")
				_ = s.Close(context.Background())
				if !errors.Is(err, question.ErrPending) || result.StopReason != run.StopReasonAwaitingInput {
					t.Fatalf("suspend=%#v %v", result, err)
				}
				questions, err := data.ListQuestions(t.Context(), "ses_history")
				if err != nil || len(questions) != 1 {
					t.Fatalf("questions=%#v %v", questions, err)
				}
				if _, err := data.ResolveQuestion(t.Context(), "ses_history", questions[0].ID, question.Reply{Status: "answered", Answers: [][]string{{"Blue"}}}); err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
					t.Fatal(err)
				}
				resumed := New(append(base, WithRunRecovery())...)
				defer resumed.Close(context.Background())
				result, err = resumed.Run(t.Context(), "go")
				if err != nil || result.Response != "done" || effects != 1 || client.calls != 2 {
					t.Fatalf("resume=%#v %v effects=%d calls=%d", result, err, effects, client.calls)
				}
				encoded, _ := json.Marshal(client.requests[1])
				if !strings.Contains(string(encoded), "Blue") {
					t.Fatal("answer missing from resumed model context")
				}
				uses, err := data.ListToolUses(t.Context(), "ses_history")
				if err != nil || len(uses) != 2 {
					t.Fatalf("uses=%#v %v", uses, err)
				}
				for _, use := range uses {
					if use.Status != store.ToolUseStatusCompleted {
						t.Fatalf("unfinished tool: %#v", use)
					}
				}
				events, err := data.(store.AggregateEventReader).ListAggregateEvents(t.Context(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_history"}, 0, 1000)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.ProjectSessionAggregate(events); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
