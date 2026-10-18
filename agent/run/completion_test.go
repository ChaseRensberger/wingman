package run

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestCompletionContinuesWithFeedbackAndTerminalHook(t *testing.T) {
	calls, validations, ends := 0, 0, 0
	client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		calls++
		if calls == 2 {
			last := req.Messages[len(req.Messages)-1]
			if last.Role != models.RoleUser || last.Content[0].(models.TextPart).Text != "fix missing test" || last.Metadata["source"] != "completion_validator" {
				t.Fatalf("feedback=%#v", last)
			}
		}
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "candidate"}}}, nil)
		return stream, nil
	}}
	result, err := Run(t.Context(), Config{Client: client, Model: testModel, Hooks: Hooks{
		BeforeComplete: func(_ context.Context, info CompletionInfo) (CompletionDecision, error) {
			validations++
			if info.Step != validations || info.Continuations != validations-1 {
				t.Fatalf("info=%#v", info)
			}
			if validations == 1 {
				return CompletionDecision{Action: "continue", Feedback: "fix missing test"}, nil
			}
			return CompletionDecision{Action: "accept"}, nil
		},
		AfterRun: func(_ context.Context, info AfterRunInfo) error {
			ends++
			if info.Err != nil || info.Result.Steps != 2 {
				t.Fatalf("end=%#v", info)
			}
			return nil
		},
	}})
	if err != nil || calls != 2 || validations != 2 || ends != 1 || result.StopReason != StopReasonEndTurn {
		t.Fatalf("calls=%d validations=%d ends=%d result=%#v err=%v", calls, validations, ends, result, err)
	}
}

func TestCompletionLimitsAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		maxSteps, maxContinuations, wantCalls int
		decision                              CompletionDecision
		hookErr                               error
		want                                  error
	}{
		{name: "default-limit", wantCalls: 4, decision: CompletionDecision{Action: "continue", Feedback: "again"}, want: ErrCompletionLimit},
		{name: "configured-limit", maxContinuations: 1, wantCalls: 2, decision: CompletionDecision{Action: "continue", Feedback: "again"}, want: ErrCompletionLimit},
		{name: "step-limit", maxSteps: 1, wantCalls: 1, decision: CompletionDecision{Action: "continue", Feedback: "again"}, want: ErrCompletionLimit},
		{name: "invalid", wantCalls: 1, decision: CompletionDecision{Action: "continue"}},
		{name: "validator-error", wantCalls: 1, hookErr: errors.New("validation failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := Run(t.Context(), Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "answer"}}}}, Model: testModel, MaxSteps: tc.maxSteps, MaxCompletionContinuations: tc.maxContinuations, Hooks: Hooks{BeforeComplete: func(context.Context, CompletionInfo) (CompletionDecision, error) {
				calls++
				return tc.decision, tc.hookErr
			}}})
			if err == nil || calls != tc.wantCalls || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

type pendingCompletion struct{}

func (pendingCompletion) Decide(context.Context, CompletionInfo, BeforeCompleteHook) (CompletionDecision, *models.Message, error) {
	return CompletionDecision{}, nil, ErrCompletionPending
}

func TestPendingCompletionDoesNotFireAfterRun(t *testing.T) {
	ends := 0
	result, err := Run(t.Context(), Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant}}, Model: testModel, CompletionLifecycle: pendingCompletion{}, Hooks: Hooks{AfterRun: func(context.Context, AfterRunInfo) error { ends++; return nil }}})
	if !errors.Is(err, ErrCompletionPending) || ends != 0 || result.StopReason != StopReasonAwaitingCompletion {
		t.Fatalf("ends=%d result=%#v err=%v", ends, result, err)
	}
}

func TestCompletionSkipsCancellationAndProviderFailure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		if canceled {
			cancel()
		}
		calls := 0
		_, err := Run(ctx, Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return nil, errors.New("provider failed")
		}}, Model: testModel, Hooks: Hooks{BeforeComplete: func(context.Context, CompletionInfo) (CompletionDecision, error) {
			calls++
			return CompletionDecision{Action: "accept"}, nil
		}}})
		cancel()
		if err == nil || calls != 0 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	}
}

func TestCompletionWaitsForToolRoundAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls, tools, validations := 0, 0, 0
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		calls++
		content := models.Content{models.TextPart{Text: "done"}}
		if calls == 1 {
			content = models.Content{models.ToolCallPart{CallID: "call", Name: "inspect"}}
		}
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: content}, nil)
		return stream, nil
	}}
	result, err := Run(ctx, Config{Model: testModel, Client: client, Tools: []tool.Tool{tool.NewFuncTool("inspect", "inspect", tool.Definition{Name: "inspect", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		tools++
		return tool.Result{Text: "ok"}, nil
	})}, Hooks: Hooks{BeforeComplete: func(_ context.Context, info CompletionInfo) (CompletionDecision, error) {
		validations++
		if info.Step != 2 || tools != 1 {
			t.Errorf("validated before tool round completed: step=%d tools=%d", info.Step, tools)
		}
		cancel()
		return CompletionDecision{Action: "continue", Feedback: "again"}, nil
	}}})
	if !errors.Is(err, context.Canceled) || calls != 2 || validations != 1 || result.StopReason != StopReasonAborted {
		t.Fatalf("calls=%d validations=%d result=%#v err=%v", calls, validations, result, err)
	}
}

func TestStructuredOutputPublishesOnlyAcceptedCandidate(t *testing.T) {
	calls := 0
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		calls++
		text := `{"answer":"rejected"}`
		if calls == 2 {
			text = `{"answer":"accepted"}`
		}
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: text}}}, nil)
		return stream, nil
	}}
	var outputs []StructuredOutputEvent
	result, err := Run(t.Context(), Config{
		Client: client, Model: testModel,
		ModelInfo:    models.ModelInfo{Capabilities: models.ModelCapabilities{StructuredOutput: true}},
		OutputSchema: &models.OutputSchema{Name: "answer", Schema: map[string]any{"type": "object"}},
		Sink: SinkFunc(func(event Event) {
			if output, ok := event.(StructuredOutputEvent); ok {
				outputs = append(outputs, output)
			}
		}),
		Hooks: Hooks{BeforeComplete: func(_ context.Context, info CompletionInfo) (CompletionDecision, error) {
			if info.Step == 1 {
				return CompletionDecision{Action: "continue", Feedback: "fix"}, nil
			}
			return CompletionDecision{Action: "accept"}, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].Parsed["answer"] != "accepted" || result.StructuredOutput["answer"] != "accepted" {
		t.Fatalf("outputs=%#v result=%#v", outputs, result)
	}
}

func TestStructuredOutputNotPublishedOnValidationFailure(t *testing.T) {
	for _, name := range []string{"error", "invalid-decision", "limit", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			outputs := 0
			result, err := Run(ctx, Config{
				Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: `{"answer":"rejected"}`}}}}, Model: testModel,
				ModelInfo:                  models.ModelInfo{Capabilities: models.ModelCapabilities{StructuredOutput: true}},
				OutputSchema:               &models.OutputSchema{Name: "answer", Schema: map[string]any{"type": "object"}},
				MaxCompletionContinuations: 1,
				Sink: SinkFunc(func(event Event) {
					if _, ok := event.(StructuredOutputEvent); ok {
						outputs++
					}
				}),
				Hooks: Hooks{BeforeComplete: func(context.Context, CompletionInfo) (CompletionDecision, error) {
					switch name {
					case "error":
						return CompletionDecision{}, errors.New("validator unavailable")
					case "invalid-decision":
						return CompletionDecision{}, nil
					case "cancellation":
						cancel()
						return CompletionDecision{Action: "accept"}, nil
					default:
						return CompletionDecision{Action: "continue", Feedback: "fix"}, nil
					}
				}},
			})
			if err == nil || outputs != 0 || result.StructuredOutput != nil {
				t.Fatalf("outputs=%d result=%#v err=%v", outputs, result, err)
			}
		})
	}
}
