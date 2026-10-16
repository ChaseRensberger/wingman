package run

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestToolOutputBoundedAfterHooksBeforePersistence(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	full := strings.Repeat("hook output\n", 10000)
	var stored ToolUseFinishInfo
	r := &runner{cfg: Config{
		WorkDir: t.TempDir(), SessionID: "session",
		Hooks: Hooks{AfterToolCall: func(_ context.Context, _ ToolCall, result ToolResult) (ToolResult, error) {
			result.Output = full
			return result, nil
		}},
		ToolUseLifecycle: toolUseLifecycleFuncs{finish: func(_ context.Context, info ToolUseFinishInfo) error { stored = info; return nil }},
	}, eventCh: make(chan Event, 1)}
	result, err := r.settleToolUse(t.Context(), ToolCall{ID: "call", Name: "tool"}, ToolResult{CallID: "call", Name: "tool", Output: "original"}, ToolUseStatusCompleted, "", nil)
	if err != nil || len(result.Output) > tool.MaxOutputBytes+512 || stored.ToolResult.Output != result.Output {
		t.Fatalf("size=%d err=%v", len(result.Output), err)
	}
	end := (<-r.eventCh).(ToolExecutionEndEvent)
	if end.Result.Output != result.Output {
		t.Fatal("unbounded final event")
	}
	path := result.Metadata["output_path"].(string)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != full {
		t.Fatalf("saved output: %v", err)
	}
}

func TestSlowProgressConsumerCanCancelWithoutLosingFinalResult(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	blocked, release, reported, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var final ToolResult
	streamer := tool.NewFuncTool("streamer", "streamer", tool.Definition{Name: "streamer", InputSchema: tool.InputSchema{Type: "object"}}, func(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
		for i := 0; i < 10000; i++ {
			inv.Progress.Report("x", nil)
		}
		close(reported)
		return tool.Result{Text: "partial result"}, ctx.Err()
	})
	go func() {
		defer close(done)
		_, _ = Run(ctx, Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "streamer", Input: map[string]any{}}}}}, Model: testModel, MaxSteps: 1, Tools: []tool.Tool{streamer}, Sink: SinkFunc(func(event Event) {
			switch event := event.(type) {
			case ToolExecutionProgressEvent:
				once.Do(func() { close(blocked); <-release })
			case ToolExecutionEndEvent:
				final = event.Result
			}
		})})
	}()
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("no progress")
	}
	cancel()
	select {
	case <-reported:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("progress blocked cancellation")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish")
	}
	if final.Output != "partial result" || !final.IsError {
		t.Fatalf("final=%#v", final)
	}
}
