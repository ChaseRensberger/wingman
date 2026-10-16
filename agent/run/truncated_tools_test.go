package run

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestTruncatedResponseDeclinesEntireToolBatch(t *testing.T) {
	for _, mode := range []ToolExecutionMode{ToolExecutionParallel, ToolExecutionSequential} {
		for _, source := range []string{"message", "finish_part", "recovery", "authorized_recovery"} {
			t.Run(string(mode)+"/"+source, func(t *testing.T) {
				var executions, before, authorizes, starts atomic.Int32
				var mu sync.Mutex
				var finishes []ToolUseFinishInfo
				message := models.Message{Role: models.RoleAssistant, FinishReason: models.FinishReasonMaxTokens, Content: models.Content{
					models.TextPart{Text: "partial answer"},
					models.ToolCallPart{ID: "part_1", CallID: "call_1", Name: "write", Input: map[string]any{"text": "complete"}},
					models.ToolCallPart{ID: "part_2", CallID: "call_2", Name: "write", Input: map[string]any{"text": "cut off"}},
				}}
				checkHistory := func(messages []models.Message) {
					t.Helper()
					if len(messages) != 1 || messages[0].FinishReason != models.FinishReasonMaxTokens || messages[0].Content[0].(models.TextPart).Text != "partial answer" {
						t.Fatalf("truncated history = %#v", messages)
					}
					for i, part := range messages[0].Content[1:] {
						p := part.(models.ToolPart)
						wantText := message.Content[i+1].(models.ToolCallPart).Input["text"]
						if source == "authorized_recovery" {
							wantText = "cut off"
						}
						if p.State != models.ToolStateError || !strings.Contains(p.Error, "output token limit") || p.Input["text"] != wantText {
							t.Fatalf("tool part = %#v", p)
						}
					}
				}
				requests := 0
				client := &modelCallTestClient{stream: func(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
					requests++
					if requests == 1 && !strings.Contains(source, "recovery") {
						if source == "finish_part" {
							final := message
							final.FinishReason = models.FinishReasonToolCalls
							stream := models.NewEventStream[models.StreamPart, *models.Message](1)
							stream.Push(models.FinishPart{Reason: models.FinishReasonMaxTokens, Usage: models.Usage{OutputTokens: 10}})
							stream.Close(&final, nil)
							return stream, nil
						}
						return completedStream(message), nil
					}
					checkHistory(request.Messages)
					return completedStream(models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}), nil
				}}
				cfg := Config{
					Client: client, Model: testModel, MaxSteps: 2, ToolExecution: mode,
					MessageCheckpoint: identifiedCheckpoint(),
					Tools: []tool.Tool{tool.NewFuncTool("write", "write", tool.Definition{Name: "write", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
						executions.Add(1)
						return tool.Result{Text: "written"}, nil
					})},
					Hooks: Hooks{BeforeToolCall: func(context.Context, ToolCall) (map[string]any, error) { before.Add(1); return nil, nil }},
					ToolUseLifecycle: toolUseLifecycleFuncs{
						propose: func(_ context.Context, info ToolUseProposeInfo) (string, error) {
							return fmt.Sprintf("tlu_%d", info.Ordinal), nil
						},
						authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorizes.Add(1); return nil },
						start:     func(context.Context, ToolUseStartInfo) error { starts.Add(1); return nil },
						finish: func(_ context.Context, info ToolUseFinishInfo) error {
							mu.Lock()
							defer mu.Unlock()
							finishes = append(finishes, info)
							return nil
						},
					},
				}
				if strings.Contains(source, "recovery") {
					cfg.Recovery = &Recovery{Step: 1, Turn: &Turn{Step: 1, Assistant: message}, Tools: map[string]RecoveredTool{}}
					if source == "authorized_recovery" {
						for i := 1; i <= 2; i++ {
							cfg.Recovery.Tools[fmt.Sprintf("part_%d", i)] = RecoveredTool{ToolUseID: fmt.Sprintf("tlu_%d", i), Args: map[string]any{"text": "cut off"}, AuthorizedAt: time.Now()}
						}
					}
				}
				result, err := Run(t.Context(), cfg)
				if err != nil {
					t.Fatal(err)
				}
				if executions.Load() != 0 || before.Load() != 0 || authorizes.Load() != 0 || starts.Load() != 0 {
					t.Fatalf("executions=%d before=%d authorizes=%d starts=%d", executions.Load(), before.Load(), authorizes.Load(), starts.Load())
				}
				if result.StopReason != StopReasonEndTurn || len(result.Turns) != 2 || len(finishes) != 2 {
					t.Fatalf("result=%#v finishes=%#v", result, finishes)
				}
				for _, finish := range finishes {
					wantStatus := ToolUseStatusDeclined
					if source == "authorized_recovery" {
						wantStatus = ToolUseStatusFailed
					}
					if finish.Status != wantStatus || finish.ErrorType != "output_token_limit" || !finish.StartedAt.IsZero() {
						t.Fatalf("finish = %#v", finish)
					}
				}
				checkHistory(result.Messages[:1])
				if source == "finish_part" && result.Usage.OutputTokens != 10 {
					t.Fatalf("usage = %#v", result.Usage)
				}
			})
		}
	}
}

func TestTruncatedToolCallCanBeReissued(t *testing.T) {
	requests, executions := 0, 0
	client := &modelCallTestClient{stream: func(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		requests++
		message := models.Message{Role: models.RoleAssistant, FinishReason: models.FinishReasonToolCalls, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "write", Input: map[string]any{"text": "complete"}}}}
		if requests == 1 {
			message.FinishReason = models.FinishReasonMaxTokens
		} else if requests == 2 {
			if part := request.Messages[0].Content[0].(models.ToolPart); part.State != models.ToolStateError {
				t.Fatalf("truncated call = %#v", part)
			}
		} else {
			if requests != 3 || request.Messages[1].Content[0].(models.ToolPart).Output != "written" {
				t.Fatalf("requests=%d history=%#v", requests, request.Messages)
			}
			message.Content = models.Content{models.TextPart{Text: "done"}}
			message.FinishReason = models.FinishReasonStop
		}
		return completedStream(message), nil
	}}
	result, err := Run(t.Context(), Config{
		Client: client, Model: testModel, MaxSteps: 3,
		Tools: []tool.Tool{tool.NewFuncTool("write", "write", tool.Definition{Name: "write", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
			executions++
			return tool.Result{Text: "written"}, nil
		})},
	})
	if err != nil || executions != 1 || requests != 3 || result.StopReason != StopReasonEndTurn {
		t.Fatalf("result=%#v err=%v executions=%d requests=%d", result, err, executions, requests)
	}
}
