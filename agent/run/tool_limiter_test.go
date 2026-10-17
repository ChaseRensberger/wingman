package run

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestToolWorkerAndSharedLimits(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprint(shared), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const count = 24
				var active atomic.Int32
				var limiter *ToolLimiter
				runs, want := 1, int32(2)
				if shared {
					limiter = NewToolLimiter(3)
					runs, want = 3, 3
				}
				release := make(chan struct{})
				done := make(chan error, runs)
				for range runs {
					go func() {
						message := models.Message{Role: models.RoleAssistant}
						for i := range count {
							message.Content = append(message.Content, models.ToolCallPart{CallID: fmt.Sprint(i), Name: "test", Input: map[string]any{"index": float64(i)}})
						}
						result, err := Run(t.Context(), Config{Model: testModel, Client: lifecycleClient{message: message}, MaxSteps: 1, MaxParallelTools: 2, ToolLimiter: limiter, Tools: []tool.Tool{tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
							if active.Add(1) > want {
								t.Error("concurrency exceeded")
							}
							<-release
							active.Add(-1)
							return tool.Result{Text: fmt.Sprint(inv.Input["index"])}, nil
						})}})
						if err == nil {
							for i, res := range result.Turns[0].Results {
								if res.Output != fmt.Sprint(i) {
									t.Error("result order changed")
								}
							}
						}
						done <- err
					}()
				}
				synctest.Wait()
				if active.Load() != want {
					t.Fatalf("active=%d want=%d", active.Load(), want)
				}
				for range count * runs {
					release <- struct{}{}
				}
				for range runs {
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				}
				if active.Load() != 0 {
					t.Fatal("leaked executions")
				}
			})
		})
	}
}

func TestSharedToolLimitFIFOAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := NewToolLimiter(1)
		if err := limiter.acquire(t.Context()); err != nil {
			t.Fatal(err)
		}
		order := make(chan int, 3)
		ctx, cancel := context.WithCancel(t.Context())
		canceled := make(chan error, 1)
		go func() { canceled <- limiter.acquire(ctx) }()
		synctest.Wait()
		for i := range 3 {
			go func() {
				if err := limiter.acquire(t.Context()); err != nil {
					t.Error(err)
					return
				}
				order <- i
				limiter.release()
			}()
			synctest.Wait()
		}
		cancel()
		if err := <-canceled; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		limiter.release()
		for i := range 3 {
			if got := <-order; got != i {
				t.Fatalf("order=%d want=%d", got, i)
			}
		}
	})
}

func TestCanceledToolAdmissionPrecedesStartedFence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := NewToolLimiter(1)
		_ = limiter.acquire(t.Context())
		ctx, cancel := context.WithCancel(t.Context())
		var starts atomic.Int32
		var finished ToolUseFinishInfo
		r := runner{eventCh: make(chan Event, 1), cfg: Config{ToolLimiter: limiter, ToolUseLifecycle: toolUseLifecycleFuncs{
			start:  func(context.Context, ToolUseStartInfo) error { starts.Add(1); return nil },
			finish: func(ctx context.Context, info ToolUseFinishInfo) error { finished = info; return ctx.Err() },
		}}}
		done := make(chan error, 1)
		go func() {
			result, err := r.executeAuthorized(ctx, ToolCall{ID: "call", ToolUseID: "use"})
			if result.Status != ToolUseStatusInterrupted || result.CallID != "call" || !result.IsError {
				t.Errorf("result=%#v", result)
			}
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if starts.Load() != 0 {
			t.Fatal("waiting tool crossed started fence")
		}
		if finished.Status != ToolUseStatusInterrupted || finished.ToolUseID != "use" || !finished.StartedAt.IsZero() {
			t.Fatalf("finish=%#v", finished)
		}
		limiter.release()
	})
}
