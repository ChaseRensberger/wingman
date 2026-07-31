package run

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestModelCallLifecycleStartPrecedesStream(t *testing.T) {
	started := make(chan struct{})
	streamEntered := make(chan struct{})
	releaseStream := make(chan struct{})
	calledBeforeStart := make(chan bool, 1)
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		select {
		case <-started:
			calledBeforeStart <- false
		default:
			calledBeforeStart <- true
		}
		close(streamEntered)
		<-releaseStream
		return completedStream(models.Message{Role: models.RoleAssistant}), nil
	}}
	lifecycle := modelCallLifecycleFuncs{
		start: func(_ context.Context, info ModelCallStartInfo) (string, error) {
			close(started)
			if info.Step != 1 || info.Attempt != 1 || info.StartedAt.IsZero() || info.Trace.Version == "" {
				t.Fatalf("start info = %#v", info)
			}
			return "call_1", nil
		},
	}
	type runResult struct {
		result *Result
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := Run(context.Background(), Config{Client: client, Model: testModel, ModelCallLifecycle: lifecycle})
		done <- runResult{result, err}
	}()
	<-streamEntered
	if <-calledBeforeStart {
		t.Fatal("Stream called before lifecycle Start")
	}
	close(releaseStream)
	out := <-done
	result, err := out.result, out.err
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns[0].ModelCallID != "call_1" || result.Turns[0].Attempt != 1 {
		t.Fatalf("turn = %#v", result.Turns[0])
	}
}

func TestModelCallLifecycleStartErrorPreventsDispatch(t *testing.T) {
	startErr := errors.New("cannot persist start")
	client := &modelCallTestClient{}
	result, err := Run(context.Background(), Config{
		Client: testClient(client), Model: testModel,
		ModelCallLifecycle: modelCallLifecycleFuncs{start: func(context.Context, ModelCallStartInfo) (string, error) {
			return "", startErr
		}},
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("error = %v, want %v", err, startErr)
	}
	if client.calls != 0 {
		t.Fatalf("Stream calls = %d, want 0", client.calls)
	}
	if len(result.Turns) != 0 {
		t.Fatalf("turns = %#v, want no physical attempt", result.Turns)
	}
}

func TestModelCallLifecycleFinishesPhysicalOutcomes(t *testing.T) {
	providerErr := errors.New("provider failed")
	for _, test := range []struct {
		name      string
		stream    func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error)
		wantErr   error
		requestID string
		cancelled bool
	}{
		{name: "success", stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return completedStream(models.Message{Role: models.RoleAssistant}), nil
		}},
		{name: "dispatch error", stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return nil, modelCallRequestError{err: providerErr, requestID: "request-failed"}
		}, wantErr: providerErr, requestID: "request-failed"},
		{name: "stream final error", stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			stream := models.NewEventStream[models.StreamPart, *models.Message](0)
			stream.Close(nil, providerErr)
			return stream, nil
		}, wantErr: providerErr},
		{name: "cancellation", stream: func(ctx context.Context, _ models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, wantErr: context.Canceled, cancelled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				go func() {
					time.Sleep(time.Millisecond)
					cancel()
				}()
			}
			var finishes []ModelCallFinishInfo
			result, err := Run(ctx, Config{
				Client: &modelCallTestClient{stream: test.stream}, Model: testModel,
				ModelCallLifecycle: modelCallLifecycleFuncs{start: func(context.Context, ModelCallStartInfo) (string, error) {
					return "call_1", nil
				}, finish: func(finishCtx context.Context, info ModelCallFinishInfo) error {
					if finishCtx.Err() != nil {
						t.Fatalf("finish context canceled: %v", finishCtx.Err())
					}
					finishes = append(finishes, info)
					return nil
				}},
			})
			if test.wantErr == nil && err != nil {
				t.Fatal(err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if len(finishes) != 1 {
				t.Fatalf("finish calls = %d, want 1", len(finishes))
			}
			finish := finishes[0]
			if finish.CallID != "call_1" || finish.Step != 1 || finish.Attempt != 1 || finish.StartedAt.IsZero() || finish.CompletedAt.IsZero() {
				t.Fatalf("finish info = %#v", finish)
			}
			if !errors.Is(finish.Failure, test.wantErr) {
				t.Fatalf("finish failure = %v, want %v", finish.Failure, test.wantErr)
			}
			if finish.ProviderRequestID != test.requestID {
				t.Fatalf("provider request ID = %q, want %q", finish.ProviderRequestID, test.requestID)
			}
			if test.wantErr == nil && finish.Assistant == nil {
				t.Fatal("successful finish has nil assistant")
			}
			if test.wantErr != nil && (len(result.Turns) != 1 || !errors.Is(result.Turns[0].Failure, test.wantErr)) {
				t.Fatalf("turns = %#v, want failed physical turn", result.Turns)
			}
		})
	}
}

func TestModelCallLifecycleJoinsFinishErrorWithPhysicalFailure(t *testing.T) {
	providerErr := errors.New("provider failed")
	finishErr := errors.New("finish failed")
	_, err := Run(context.Background(), Config{
		Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return nil, providerErr
		}},
		Model: testModel,
		ModelCallLifecycle: modelCallLifecycleFuncs{
			start:  func(context.Context, ModelCallStartInfo) (string, error) { return "call_1", nil },
			finish: func(context.Context, ModelCallFinishInfo) error { return finishErr },
		},
	})
	if !errors.Is(err, providerErr) || !errors.Is(err, finishErr) {
		t.Fatalf("error = %v, want both physical and finish errors", err)
	}
}

func TestModelCallRetriesDispatchFailuresAsDistinctPhysicalAttempts(t *testing.T) {
	providerErr := &models.ProviderError{Category: models.ErrorRateLimit, Provider: "test", RequestID: "req_1", Retryable: true, Message: "rate limited"}
	var calls int
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		calls++
		if calls == 1 {
			return nil, providerErr
		}
		return completedStream(models.Message{Role: models.RoleAssistant}), nil
	}}
	var starts []ModelCallStartInfo
	var finishes []ModelCallFinishInfo
	result, err := Run(context.Background(), Config{
		Client: client, Model: testModel,
		Retry: RetryPolicy{MaxAttempts: 2, InitialDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
		ModelCallLifecycle: modelCallLifecycleFuncs{
			start: func(_ context.Context, info ModelCallStartInfo) (string, error) {
				starts = append(starts, info)
				return fmt.Sprintf("call_%d", info.Attempt), nil
			},
			finish: func(_ context.Context, info ModelCallFinishInfo) error {
				finishes = append(finishes, info)
				return nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(starts) != 2 || len(finishes) != 2 {
		t.Fatalf("calls = %d, starts = %#v, finishes = %#v", calls, starts, finishes)
	}
	if starts[0].Attempt != 1 || starts[1].Attempt != 2 || finishes[0].Attempt != 1 || finishes[1].Attempt != 2 {
		t.Fatalf("starts = %#v, finishes = %#v", starts, finishes)
	}
	if finishes[0].ProviderRequestID != "req_1" || !errors.Is(finishes[0].Failure, providerErr) || finishes[0].Assistant != nil {
		t.Fatalf("first finish = %#v", finishes[0])
	}
	if len(result.Turns) != 1 || result.Turns[0].Attempt != 2 || result.Turns[0].ModelCallID != "call_2" {
		t.Fatalf("turns = %#v", result.Turns)
	}
}

func TestModelCallRetryRequiresSettlementAndHonorsCancellation(t *testing.T) {
	providerErr := &models.ProviderError{Category: models.ErrorUnavailable, Retryable: true, Message: "unavailable"}
	for _, test := range []struct {
		name       string
		finishErr  error
		cancelWait bool
		wantErr    error
	}{
		{name: "settlement failure", finishErr: errors.New("settlement failed")},
		{name: "cancel backoff", cancelWait: true, wantErr: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
				calls++
				if test.cancelWait {
					cancel()
				}
				return nil, providerErr
			}}
			_, err := Run(ctx, Config{
				Client: client, Model: testModel,
				Retry: RetryPolicy{MaxAttempts: 3, InitialDelay: time.Hour, MaxDelay: time.Hour},
				ModelCallLifecycle: modelCallLifecycleFuncs{
					start:  func(context.Context, ModelCallStartInfo) (string, error) { return "call_1", nil },
					finish: func(context.Context, ModelCallFinishInfo) error { return test.finishErr },
				},
			})
			if calls != 1 {
				t.Fatalf("Stream calls = %d, want 1", calls)
			}
			if test.finishErr != nil && !errors.Is(err, test.finishErr) {
				t.Fatalf("error = %v, want settlement failure", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestModelCallDoesNotRetryNonRetryableOrEstablishedStreamFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error)
	}{
		{name: "non-retryable dispatch", stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return nil, &models.ProviderError{Category: models.ErrorInvalidRequest, Message: "invalid"}
		}},
		{name: "established stream", stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			stream := models.NewEventStream[models.StreamPart, *models.Message](0)
			stream.Close(nil, &models.ProviderError{Category: models.ErrorTransport, Retryable: true, Message: "reset"})
			return stream, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &modelCallTestClient{stream: test.stream}
			_, err := Run(context.Background(), Config{Client: client, Model: testModel, Retry: RetryPolicy{MaxAttempts: 3, InitialDelay: time.Nanosecond}})
			if err == nil {
				t.Fatal("Run succeeded")
			}
			if client.calls != 1 {
				t.Fatalf("Stream calls = %d, want 1", client.calls)
			}
		})
	}
}

func TestRetryDelayHonorsProviderRecommendationAndCapsBackoff(t *testing.T) {
	recommended := 7 * time.Second
	policy := RetryPolicy{InitialDelay: time.Second, MaxDelay: 2 * time.Second}
	if got := retryDelay(policy, 1, &models.ProviderError{RetryAfter: &recommended}); got != recommended {
		t.Fatalf("recommended delay = %v, want %v", got, recommended)
	}
	if got := retryDelay(policy, 3, errors.New("transport")); got != 2*time.Second {
		t.Fatalf("backoff = %v, want 2s", got)
	}
}

func TestModelCallLifecyclePropagatesRequestIDAndFinishesBeforeTools(t *testing.T) {
	var order []string
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		go func() {
			stream.Push(models.ResponseMetadataPart{Meta: map[string]any{"request_id": "provider_1"}})
			stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "tool_1", Name: "test", Input: map[string]any{}}}}, nil)
		}()
		return stream, nil
	}}
	testTool := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		order = append(order, "tool")
		return tool.Result{}, nil
	})
	result, err := Run(context.Background(), Config{
		Client: client, Model: testModel, Tools: []tool.Tool{testTool}, MaxSteps: 1,
		ModelCallLifecycle: modelCallLifecycleFuncs{start: func(context.Context, ModelCallStartInfo) (string, error) {
			return "call_1", nil
		}, finish: func(_ context.Context, info ModelCallFinishInfo) error {
			order = append(order, "finish")
			if info.ProviderRequestID != "provider_1" {
				t.Fatalf("request ID = %q", info.ProviderRequestID)
			}
			return nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Turns[0].ProviderRequestID != "provider_1" || len(order) != 2 || order[0] != "finish" || order[1] != "tool" {
		t.Fatalf("turn = %#v, order = %#v", result.Turns[0], order)
	}
}

func TestToolHookFailureDoesNotFailPhysicalModelCall(t *testing.T) {
	var finish ModelCallFinishInfo
	result, err := Run(context.Background(), Config{
		Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "tool_1", Name: "test", Input: map[string]any{}}}}},
		Model:  testModel,
		Tools:  []tool.Tool{tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) { return tool.Result{}, nil })},
		Hooks:  Hooks{BeforeToolCall: func(context.Context, ToolCall) (map[string]any, error) { return nil, errors.New("tool hook failed") }},
		ModelCallLifecycle: modelCallLifecycleFuncs{start: func(context.Context, ModelCallStartInfo) (string, error) { return "call_1", nil }, finish: func(_ context.Context, info ModelCallFinishInfo) error {
			finish = info
			return nil
		}},
	})
	if err == nil {
		t.Fatal("Run succeeded, want tool hook error")
	}
	if len(result.Turns) != 1 || result.Turns[0].Failure != nil || finish.Failure != nil {
		t.Fatalf("turns = %#v, finish = %#v", result.Turns, finish)
	}
}

var testModel = models.ModelRef{Provider: "test", ID: "model"}

type modelCallLifecycleFuncs struct {
	start  func(context.Context, ModelCallStartInfo) (string, error)
	finish func(context.Context, ModelCallFinishInfo) error
}

type modelCallRequestError struct {
	err       error
	requestID string
}

func (e modelCallRequestError) Error() string             { return e.err.Error() }
func (e modelCallRequestError) Unwrap() error             { return e.err }
func (e modelCallRequestError) ProviderRequestID() string { return e.requestID }

func (f modelCallLifecycleFuncs) Start(ctx context.Context, info ModelCallStartInfo) (string, error) {
	if f.start == nil {
		return "", nil
	}
	return f.start(ctx, info)
}

func (f modelCallLifecycleFuncs) Finish(ctx context.Context, info ModelCallFinishInfo) error {
	if f.finish == nil {
		return nil
	}
	return f.finish(ctx, info)
}

type modelCallTestClient struct {
	mu     sync.Mutex
	calls  int
	stream func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error)
}

func testClient(client *modelCallTestClient) models.Client { return client }

func (c *modelCallTestClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *modelCallTestClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *modelCallTestClient) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if c.stream == nil {
		return nil, errors.New("unexpected Stream")
	}
	return c.stream(ctx, req)
}

func completedStream(message models.Message) *models.EventStream[models.StreamPart, *models.Message] {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&message, nil)
	return stream
}
