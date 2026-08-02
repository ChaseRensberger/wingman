package run

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestExecuteOneEmitsProgressEvents(t *testing.T) {
	t.Parallel()

	var events []Event
	var mu sync.Mutex
	sink := SinkFunc(func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	streamingTool := tool.NewFuncTool("streamer", "streams", tool.Definition{
		Name:        "streamer",
		Description: "streams",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
		if inv.Progress != nil {
			inv.Progress.Report("chunk1", map[string]any{"k": "v"})
			inv.Progress.Report("chunk2", nil)
		}
		return tool.Result{Text: "chunk1chunk2"}, nil
	})

	r := &runner{
		cfg: Config{
			WorkDir: t.TempDir(),
			Sink:    sink,
		},
		eventCh: make(chan Event, 64),
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range r.eventCh {
			if r.cfg.Sink != nil {
				r.cfg.Sink.OnEvent(ev)
			}
		}
	}()

	_, err := r.executeOne(context.Background(), ToolCall{
		ID:   "call_1",
		Name: "streamer",
		Args: map[string]any{},
		Tool: streamingTool,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(r.eventCh)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	var startCount, progressCount, endCount int
	for _, e := range events {
		switch v := e.(type) {
		case ToolExecutionStartEvent:
			startCount++
			if v.Call.ID != "call_1" {
				t.Fatalf("start event call_id = %s", v.Call.ID)
			}
		case ToolExecutionProgressEvent:
			progressCount++
			if v.CallID != "call_1" || v.Name != "streamer" {
				t.Fatalf("progress event = %#v", v)
			}
			if v.OutputDelta != "chunk1" && v.OutputDelta != "chunk2" {
				t.Fatalf("unexpected delta %q", v.OutputDelta)
			}
			if v.OutputDelta == "chunk1" {
				if v.Metadata == nil || v.Metadata["k"] != "v" {
					t.Fatalf("progress metadata = %#v", v.Metadata)
				}
			}
		case ToolExecutionEndEvent:
			endCount++
			if v.Result.CallID != "call_1" || v.Result.Output != "chunk1chunk2" {
				t.Fatalf("end event result = %#v", v.Result)
			}
		}
	}

	if startCount != 1 || progressCount != 2 || endCount != 1 {
		t.Fatalf("events: start=%d progress=%d end=%d", startCount, progressCount, endCount)
	}
}

func TestExecuteOnePreservesPartialOutputOnToolError(t *testing.T) {
	t.Parallel()

	var events []Event
	var mu sync.Mutex
	sink := SinkFunc(func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	failingTool := tool.NewFuncTool("failer", "fails", tool.Definition{
		Name:        "failer",
		Description: "fails",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(_ context.Context, _ tool.Invocation) (tool.Result, error) {
		return tool.Result{Text: "partial output"}, &testError{msg: "something went wrong"}
	})

	r := &runner{
		cfg: Config{
			WorkDir: t.TempDir(),
			Sink:    sink,
		},
		eventCh: make(chan Event, 64),
	}
	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for ev := range r.eventCh {
			if r.cfg.Sink != nil {
				r.cfg.Sink.OnEvent(ev)
			}
		}
	}()

	_, err := r.executeOne(context.Background(), ToolCall{
		ID:   "call_1",
		Name: "failer",
		Args: map[string]any{},
		Tool: failingTool,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(r.eventCh)
	wg2.Wait()

	mu.Lock()
	defer mu.Unlock()

	for _, e := range events {
		if end, ok := e.(ToolExecutionEndEvent); ok {
			if end.Result.Output != "partial output" {
				t.Fatalf("result.Output = %q, want partial output", end.Result.Output)
			}
			if end.Result.Error != "something went wrong" {
				t.Fatalf("result.Error = %q, want error text", end.Result.Error)
			}
			if !end.Result.IsError {
				t.Fatal("expected IsError=true")
			}
			return
		}
	}
	t.Fatal("no ToolExecutionEndEvent found")
}

func TestAfterToolCallPreservesStructuredError(t *testing.T) {
	r := &runner{
		cfg: Config{Hooks: Hooks{AfterToolCall: func(_ context.Context, _ ToolCall, result ToolResult) (ToolResult, error) {
			result.Metadata = map[string]any{"observed": true}
			return result, nil
		}},
		},
	}
	input := ToolResult{CallID: "call_1", Output: "partial", Error: "failed", IsError: true}
	result := r.runAfterToolCall(context.Background(), ToolCall{ID: "call_1"}, input)
	if result.Output != "partial" || result.Error != "failed" || !result.IsError {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["observed"] != true {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestBuildToolResultMessagePreservesOutputParts(t *testing.T) {
	message := buildToolResultMessage([]ToolResult{{
		CallID:      "call_1",
		Name:        "screenshot",
		Output:      "captured",
		OutputParts: models.Content{models.ImagePart{Base64: "image-data", MediaType: "image/png"}},
		Structured:  map[string]any{"width": 10},
	}})

	result := message.Content[0].(models.ToolResultPart)
	if len(result.Output) != 2 {
		t.Fatalf("output = %#v, want text and image", result.Output)
	}
	if _, ok := result.Output[1].(models.ImagePart); !ok {
		t.Fatalf("output[1] = %T, want ImagePart", result.Output[1])
	}
	if result.Structured.(map[string]any)["width"] != 10 {
		t.Fatalf("structured = %#v", result.Structured)
	}
}

func TestAfterToolCallPreservesResultIdentity(t *testing.T) {
	r := &runner{cfg: Config{Hooks: Hooks{AfterToolCall: func(_ context.Context, _ ToolCall, _ ToolResult) (ToolResult, error) {
		return ToolResult{Output: "rewritten"}, nil
	}}}}
	input := ToolResult{CallID: "call_1", Name: "tool", Args: map[string]any{"value": true}, Duration: time.Second}
	result := r.runAfterToolCall(context.Background(), ToolCall{ID: "call_1"}, input)
	if result.CallID != input.CallID || result.Name != input.Name || result.Args["value"] != true || result.Duration != input.Duration {
		t.Fatalf("result identity = %#v", result)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func TestRunSinkClassifiesToolProgress(t *testing.T) {
	t.Parallel()
	client := lifecycleClient{message: models.Message{
		Role: models.RoleAssistant,
		Content: models.Content{models.ToolCallPart{
			CallID: "call_1",
			Name:   "noop",
			Input:  map[string]any{},
		}},
	}}

	streamingTool := tool.NewFuncTool("noop", "no op", tool.Definition{
		Name:        "noop",
		Description: "no op",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
		if inv.Progress != nil {
			inv.Progress.Report("delta", map[string]any{"step": 1})
		}
		return tool.Result{Text: "ok"}, nil
	})

	// We test classification indirectly by verifying the runner emits the
	// right event type; the session stream layer maps it to "tool_progress".
	var found bool
	var mu sync.Mutex
	sink := SinkFunc(func(e Event) {
		if p, ok := e.(ToolExecutionProgressEvent); ok {
			mu.Lock()
			found = true
			if p.CallID != "call_1" || p.Name != "noop" || p.OutputDelta != "delta" {
				t.Fatalf("progress = %#v", p)
			}
			mu.Unlock()
		}
	})

	_, _ = Run(context.Background(), Config{
		Client:   client,
		Model:    models.ModelRef{Provider: "test", ID: "model"},
		Tools:    []tool.Tool{streamingTool},
		Sink:     sink,
		MaxSteps: 1,
	})

	mu.Lock()
	defer mu.Unlock()
	if !found {
		t.Fatal("expected ToolExecutionProgressEvent")
	}
}
