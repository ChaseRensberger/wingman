package run

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

func TestRunRetainsToolCallWhenBeforeToolCallFails(t *testing.T) {
	t.Parallel()
	client := lifecycleClient{message: models.Message{
		Role: models.RoleAssistant,
		Content: models.Content{models.ToolCallPart{
			CallID: "call_1",
			Name:   "test",
			Input:  map[string]any{},
		}},
	}}
	result, err := Run(context.Background(), Config{
		Client: client,
		Model:  models.ModelRef{Provider: "test", ID: "model"},
		Tools: []tool.Tool{tool.NewFuncTool("test", "test", tool.Definition{
			Name:        "test",
			Description: "test",
			InputSchema: tool.InputSchema{Type: "object"},
		}, func(context.Context, tool.Invocation) (tool.Result, error) {
			return tool.Result{}, nil
		})},
		Hooks: Hooks{BeforeToolCall: func(context.Context, ToolCall) (map[string]any, error) {
			return nil, errors.New("blocked")
		}},
	})
	if err == nil {
		t.Fatal("Run succeeded, want hook error")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %#v, want retained assistant message", result.Messages)
	}
	if len(result.Turns) != 1 {
		t.Fatalf("turns = %#v, want retained partial turn", result.Turns)
	}
	part, ok := result.Messages[0].Content[0].(models.ToolPart)
	if !ok {
		t.Fatalf("part = %T, want ToolPart", result.Messages[0].Content[0])
	}
	if part.State != models.ToolStateError || part.CallID != "call_1" || part.Error != "blocked" {
		t.Fatalf("tool part = %#v, want failed call_1", part)
	}
}

type lifecycleClient struct {
	message models.Message
}

func (c lifecycleClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c lifecycleClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func TestRunTurnRecordsTimingAndTrace(t *testing.T) {
	t.Parallel()
	client := &timingTestClient{}
	result, err := Run(context.Background(), Config{
		Client: client,
		Model:  models.ModelRef{Provider: "test", ID: "model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(result.Turns))
	}
	turn := result.Turns[0]
	if turn.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero")
	}
	if turn.CompletedAt.IsZero() {
		t.Fatal("CompletedAt is zero")
	}
	if turn.CompletedAt.Before(turn.StartedAt) {
		t.Fatal("CompletedAt before StartedAt")
	}
	if turn.Trace.Version != "1" {
		t.Fatalf("trace version = %q, want 1", turn.Trace.Version)
	}
	if turn.Trace.Provider != "test" {
		t.Fatalf("trace provider = %q, want test", turn.Trace.Provider)
	}
}

func TestRunRetainsStartedFailedTurn(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		client *failingStreamClient
	}{
		{name: "stream", client: &failingStreamClient{streamErr: errors.New("stream failed")}},
		{name: "final", client: &failingStreamClient{finalErr: errors.New("stream final failed")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Run(context.Background(), Config{
				Client: test.client,
				Model:  models.ModelRef{Provider: "test", ID: "model"},
			})
			if err == nil {
				t.Fatal("Run succeeded, want stream error")
			}
			if len(result.Turns) != 1 {
				t.Fatalf("turns = %d, want 1", len(result.Turns))
			}
			turn := result.Turns[0]
			if turn.Failure == nil || turn.StartedAt.IsZero() || turn.CompletedAt.IsZero() || turn.Trace.Version != "1" {
				t.Fatalf("turn = %#v, want failed traced attempt", turn)
			}
		})
	}
}

type timingTestClient struct{}

type failingStreamClient struct {
	streamErr error
	finalErr  error
}

func (c *timingTestClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return &models.PreparedRequest{Model: models.ModelRef{Provider: "test", ID: "model"}}, nil
}

func (c *timingTestClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *timingTestClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "ok"}}}, nil)
	return stream, nil
}

func (c *failingStreamClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *failingStreamClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *failingStreamClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	if c.streamErr != nil {
		return nil, c.streamErr
	}
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(nil, c.finalErr)
	return stream, nil
}

func (c lifecycleClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&c.message, nil)
	return stream, nil
}

func TestToolUseLifecycleCheckpointsProposalBeforeExecution(t *testing.T) {
	var order []string
	var executed int
	var checkpointedToolUse bool
	lifecycle := toolUseLifecycleFuncs{
		propose: func(_ context.Context, info ToolUseProposeInfo) (string, error) {
			order = append(order, "propose")
			if info.Ordinal != 1 || info.PartID != "part_1" || info.MessageID != "message_1" {
				t.Fatalf("proposal identity = %#v", info)
			}
			return "tlu_1", nil
		},
		authorize: func(_ context.Context, info ToolUseAuthorizeInfo) error {
			order = append(order, "authorize")
			if info.Args["rewritten"] != true {
				t.Fatalf("authorized args = %#v", info.Args)
			}
			return nil
		},
		start: func(context.Context, ToolUseStartInfo) error { order = append(order, "start"); return nil },
		finish: func(_ context.Context, info ToolUseFinishInfo) error {
			order = append(order, "finish")
			if info.Status != ToolUseStatusCompleted || info.ToolResult.ToolUseID != "tlu_1" {
				t.Fatalf("finish = %#v", info)
			}
			return nil
		},
	}
	result, err := Run(context.Background(), Config{
		Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}},
		Model:  testModel, MaxSteps: 1, MessageCheckpoint: checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
			message := info.Message
			if message.ID == "" {
				message.ID = "message_1"
			}
			for i, part := range message.Content {
				if models.PartID(part) == "" {
					message.Content[i] = models.WithPartID(part, "part_1")
				}
				if toolPart, ok := message.Content[i].(models.ToolPart); ok && toolPart.ToolUseID == "tlu_1" {
					checkpointedToolUse = true
				}
			}
			return message, nil
		}), ToolUseLifecycle: lifecycle,
		Tools: []tool.Tool{tool.NewFuncTool("test", "test", tool.Definition{Name: "test", Description: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
			if !checkpointedToolUse {
				t.Fatal("tool executed before tool-use ID checkpoint")
			}
			order = append(order, "execute")
			executed++
			return tool.Result{Text: "ok"}, nil
		})},
		Hooks: Hooks{BeforeToolCall: func(_ context.Context, call ToolCall) (map[string]any, error) {
			return map[string]any{"rewritten": true}, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed != 1 || len(order) != 5 || order[0] != "propose" || order[1] != "authorize" || order[2] != "start" || order[3] != "execute" || order[4] != "finish" {
		t.Fatalf("order = %#v", order)
	}
	part := result.Messages[0].Content[0].(models.ToolPart)
	if part.ToolUseID != "tlu_1" || part.Input["rewritten"] != true {
		t.Fatalf("canonical tool = %#v", part)
	}
}

func TestToolUseLifecycleTransitionFailuresRetainTerminalResult(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		fail                        string
		wantExecutions              int
		wantAuthorized, wantStarted bool
	}{
		{name: "authorize", fail: "authorize"},
		{name: "start", fail: "start", wantAuthorized: true},
		{name: "finish", fail: "finish", wantExecutions: 1, wantAuthorized: true, wantStarted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var executions int
			var finished ToolUseFinishInfo
			lifecycle := toolUseLifecycleFuncs{
				propose: func(context.Context, ToolUseProposeInfo) (string, error) { return "tlu_1", nil },
				authorize: func(context.Context, ToolUseAuthorizeInfo) error {
					if tc.fail == "authorize" {
						return errors.New("authorize failed")
					}
					return nil
				},
				start: func(context.Context, ToolUseStartInfo) error {
					if tc.fail == "start" {
						return errors.New("start failed")
					}
					return nil
				},
				finish: func(_ context.Context, info ToolUseFinishInfo) error {
					finished = info
					if tc.fail == "finish" {
						return errors.New("finish failed")
					}
					return nil
				},
			}
			result, err := Run(context.Background(), Config{
				Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}},
				Model:  testModel, MaxSteps: 1, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle,
				Tools: []tool.Tool{tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) { executions++; return tool.Result{}, nil })},
			})
			if err == nil || executions != tc.wantExecutions {
				t.Fatalf("err=%v executions=%d", err, executions)
			}
			if finished.AuthorizedAt.IsZero() != !tc.wantAuthorized || finished.StartedAt.IsZero() != !tc.wantStarted {
				t.Fatalf("timestamps = %#v", finished)
			}
			part := result.Messages[0].Content[0].(models.ToolPart)
			if part.State != models.ToolStateError || part.ToolUseID != "tlu_1" {
				t.Fatalf("part = %#v", part)
			}
		})
	}
}

func TestToolUseLifecycleDeclinesBeforeAuthorization(t *testing.T) {
	validTool := func(executions *int) tool.Tool {
		return tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object", Required: []string{"required"}}}, func(context.Context, tool.Invocation) (tool.Result, error) { *executions++; return tool.Result{}, nil })
	}
	for _, tc := range []struct {
		name, toolName, errorType string
		input                     map[string]any
		permissions               permission.Ruleset
		skip                      bool
	}{
		{name: "unknown", toolName: "unknown", errorType: "unknown_tool", input: map[string]any{}},
		{name: "invalid", toolName: "test", errorType: "input_validation", input: map[string]any{}},
		{name: "deny", toolName: "test", errorType: "permission_denied", input: map[string]any{"required": true}, permissions: permission.Ruleset{{Action: "test", Resource: "*", Effect: permission.EffectDeny}}},
		{name: "ask", toolName: "test", errorType: "permission_unavailable", input: map[string]any{"required": true}, permissions: permission.Ruleset{{Action: "test", Resource: "*", Effect: permission.EffectAsk}}},
		{name: "skip", toolName: "test", errorType: "tool_skipped", input: map[string]any{"required": true}, skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var executions, authorizes, starts int
			var finished ToolUseFinishInfo
			lifecycle := toolUseLifecycleFuncs{
				propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "tlu_1", nil },
				authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorizes++; return nil },
				start:     func(context.Context, ToolUseStartInfo) error { starts++; return nil },
				finish:    func(_ context.Context, info ToolUseFinishInfo) error { finished = info; return nil },
			}
			cfg := Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: tc.toolName, Input: tc.input}}}}, Model: testModel, MaxSteps: 1, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle, Permissions: tc.permissions}
			if tc.toolName == "test" {
				cfg.Tools = []tool.Tool{validTool(&executions)}
			}
			if tc.skip {
				cfg.Hooks.BeforeToolCall = func(context.Context, ToolCall) (map[string]any, error) {
					return map[string]any{"replacement": true}, ErrSkipTool
				}
			}
			result, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			if executions != 0 || authorizes != 0 || starts != 0 || finished.Status != ToolUseStatusDeclined || finished.ErrorType != tc.errorType {
				t.Fatalf("executions=%d authorize=%d start=%d finish=%#v", executions, authorizes, starts, finished)
			}
			part := result.Messages[0].Content[0].(models.ToolPart)
			if part.State != models.ToolStateError || part.ToolUseID != "tlu_1" {
				t.Fatalf("part = %#v", part)
			}
		})
	}
}

func TestToolUseProposalFailureInterruptsEarlierProposals(t *testing.T) {
	var executions, authorizes, starts int
	var finished []ToolUseFinishInfo
	var terminalEvents []ToolResult
	lifecycle := toolUseLifecycleFuncs{
		propose: func(_ context.Context, info ToolUseProposeInfo) (string, error) {
			if info.Ordinal == 1 {
				return "tlu_1", nil
			}
			return "", errors.New("second proposal failed")
		},
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorizes++; return nil },
		start:     func(context.Context, ToolUseStartInfo) error { starts++; return nil },
		finish: func(ctx context.Context, info ToolUseFinishInfo) error {
			if ctx.Err() != nil {
				t.Errorf("finish context canceled: %v", ctx.Err())
			}
			finished = append(finished, info)
			return nil
		},
	}
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executions++
		return tool.Result{}, nil
	})
	result, err := Run(context.Background(), Config{
		Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{
			models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}},
			models.ToolCallPart{CallID: "call_2", Name: "test", Input: map[string]any{}},
		}}},
		Model: testModel, Tools: []tool.Tool{toolSpy}, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle,
		Sink: SinkFunc(func(event Event) {
			if end, ok := event.(ToolExecutionEndEvent); ok {
				terminalEvents = append(terminalEvents, end.Result)
			}
		}),
	})
	if err == nil {
		t.Fatal("Run succeeded, want proposal error")
	}
	if executions != 0 || authorizes != 0 || starts != 0 || len(finished) != 1 {
		t.Fatalf("executions=%d authorizes=%d starts=%d finishes=%#v", executions, authorizes, starts, finished)
	}
	if finished[0].ToolUseID != "tlu_1" || finished[0].Status != ToolUseStatusInterrupted {
		t.Fatalf("finish = %#v", finished[0])
	}
	if len(terminalEvents) != 1 || terminalEvents[0].ToolUseID != "tlu_1" || terminalEvents[0].Status != ToolUseStatusInterrupted {
		t.Fatalf("terminal events = %#v", terminalEvents)
	}
	first := result.Messages[0].Content[0].(models.ToolPart)
	second := result.Messages[0].Content[1].(models.ToolPart)
	if first.ToolUseID != "tlu_1" || first.State != models.ToolStateError || second.ToolUseID != "" || second.State != models.ToolStatePending {
		t.Fatalf("parts = %#v", result.Messages[0].Content)
	}
}

func TestToolUseProposalCheckpointFailureInterruptsAllProposals(t *testing.T) {
	var executions, authorizes, starts int
	var finished []ToolUseFinishInfo
	var failedSnapshot models.Message
	proposalCheckpointFailed := false
	lifecycle := toolUseLifecycleFuncs{
		propose: func(_ context.Context, info ToolUseProposeInfo) (string, error) {
			return "tlu_" + string(rune('0'+info.Ordinal)), nil
		},
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorizes++; return nil },
		start:     func(context.Context, ToolUseStartInfo) error { starts++; return nil },
		finish: func(ctx context.Context, info ToolUseFinishInfo) error {
			if ctx.Err() != nil {
				t.Errorf("finish context canceled: %v", ctx.Err())
			}
			finished = append(finished, info)
			return nil
		},
	}
	checkpoint := checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
		message := info.Message
		if message.ID == "" {
			message.ID = "message_1"
		}
		for i, part := range message.Content {
			if models.PartID(part) == "" {
				message.Content[i] = models.WithPartID(part, "part_"+string(rune('1'+i)))
			}
		}
		if len(message.Content) == 2 {
			first, firstOK := message.Content[0].(models.ToolPart)
			second, secondOK := message.Content[1].(models.ToolPart)
			if firstOK && secondOK && first.ToolUseID != "" && second.ToolUseID != "" && !proposalCheckpointFailed {
				proposalCheckpointFailed = true
				return models.Message{}, errors.New("proposal checkpoint failed")
			}
			if message.State == models.MessageStateFailed {
				failedSnapshot = message
			}
		}
		return message, nil
	})
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executions++
		return tool.Result{}, nil
	})
	result, err := Run(context.Background(), Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{
		models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}},
		models.ToolCallPart{CallID: "call_2", Name: "test", Input: map[string]any{}},
	}}}, Model: testModel, Tools: []tool.Tool{toolSpy}, MessageCheckpoint: checkpoint, ToolUseLifecycle: lifecycle})
	if err == nil || !proposalCheckpointFailed {
		t.Fatalf("err=%v proposal checkpoint failed=%v", err, proposalCheckpointFailed)
	}
	if executions != 0 || authorizes != 0 || starts != 0 || len(finished) != 2 {
		t.Fatalf("executions=%d authorizes=%d starts=%d finishes=%#v", executions, authorizes, starts, finished)
	}
	for _, info := range finished {
		if info.Status != ToolUseStatusInterrupted {
			t.Fatalf("finish = %#v", info)
		}
	}
	if len(failedSnapshot.Content) != 2 || failedSnapshot.State != models.MessageStateFailed {
		t.Fatalf("failed snapshot = %#v", failedSnapshot)
	}
	for _, part := range failedSnapshot.Content {
		toolPart := part.(models.ToolPart)
		if toolPart.ToolUseID == "" || toolPart.State != models.ToolStateError {
			t.Fatalf("failed tool part = %#v", toolPart)
		}
	}
	if result.Messages[0].State != models.MessageStateFailed {
		t.Fatalf("result message = %#v", result.Messages[0])
	}
}

func TestToolExecutionCancellationIsInterruptedAndSettled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var executions int
	var finished ToolUseFinishInfo
	lifecycle := toolUseLifecycleFuncs{
		propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "tlu_1", nil },
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { return nil },
		start:     func(context.Context, ToolUseStartInfo) error { return nil },
		finish: func(finishCtx context.Context, info ToolUseFinishInfo) error {
			if finishCtx.Err() != nil {
				t.Errorf("finish context canceled: %v", finishCtx.Err())
			}
			finished = info
			return nil
		},
	}
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(execCtx context.Context, _ tool.Invocation) (tool.Result, error) {
		executions++
		cancel()
		return tool.Result{}, execCtx.Err()
	})
	result, err := Run(ctx, Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}}, Model: testModel, Tools: []tool.Tool{toolSpy}, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle})
	if !errors.Is(err, context.Canceled) || executions != 1 {
		t.Fatalf("err=%v executions=%d", err, executions)
	}
	if finished.Status != ToolUseStatusInterrupted || !finished.ToolResult.IsError || !errors.Is(finished.Failure, context.Canceled) {
		t.Fatalf("finish = %#v", finished)
	}
	if len(result.Turns) != 1 || result.Turns[0].Results[0].Status != ToolUseStatusInterrupted {
		t.Fatalf("results = %#v", result.Turns)
	}
	part := result.Messages[0].Content[0].(models.ToolPart)
	if part.ToolUseID != "tlu_1" || part.State != models.ToolStateError {
		t.Fatalf("part = %#v", part)
	}
}

func TestToolLifecycleEventsShareToolUseID(t *testing.T) {
	var mu sync.Mutex
	var events []Event
	startReturned := false
	startObservedAfterReturn := false
	lifecycle := toolUseLifecycleFuncs{
		propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "tlu_1", nil },
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { return nil },
		start: func(context.Context, ToolUseStartInfo) error {
			mu.Lock()
			startReturned = true
			mu.Unlock()
			return nil
		},
		finish: func(context.Context, ToolUseFinishInfo) error { return nil },
	}
	streamingTool := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
		inv.Progress.Report("progress", nil)
		return tool.Result{Text: "ok"}, nil
	})
	_, err := Run(context.Background(), Config{
		Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}},
		Model:  testModel, Tools: []tool.Tool{streamingTool}, MaxSteps: 1, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle,
		Sink: SinkFunc(func(event Event) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
			if _, ok := event.(ToolExecutionStartEvent); ok {
				startObservedAfterReturn = startReturned
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var proposed, authorized, started, progressed, ended int
	mu.Lock()
	defer mu.Unlock()
	for _, event := range events {
		switch event := event.(type) {
		case ToolUseProposedEvent:
			proposed++
			if event.Call.ToolUseID != "tlu_1" {
				t.Fatalf("proposed = %#v", event)
			}
		case ToolUseAuthorizedEvent:
			authorized++
			if event.Call.ToolUseID != "tlu_1" {
				t.Fatalf("authorized = %#v", event)
			}
		case ToolExecutionStartEvent:
			started++
			if event.Call.ToolUseID != "tlu_1" {
				t.Fatalf("started = %#v", event)
			}
		case ToolExecutionProgressEvent:
			progressed++
			if event.ToolUseID != "tlu_1" {
				t.Fatalf("progress = %#v", event)
			}
		case ToolExecutionEndEvent:
			ended++
			if event.Result.ToolUseID != "tlu_1" || event.Result.Status != ToolUseStatusCompleted {
				t.Fatalf("ended = %#v", event)
			}
		}
	}
	if proposed != 1 || authorized != 1 || started != 1 || progressed != 1 || ended != 1 || !startObservedAfterReturn {
		t.Fatalf("events proposed=%d authorized=%d started=%d progressed=%d ended=%d start after lifecycle=%v", proposed, authorized, started, progressed, ended, startObservedAfterReturn)
	}
}

func TestParallelToolUsesRemainAssociatedByPartAndToolUseID(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	secondFinished := make(chan struct{})
	var mu sync.Mutex
	var finishes []ToolUseFinishInfo
	var proposalOrdinals []int
	lifecycle := toolUseLifecycleFuncs{
		propose: func(_ context.Context, info ToolUseProposeInfo) (string, error) {
			proposalOrdinals = append(proposalOrdinals, info.Ordinal)
			return "tlu_" + string(rune('0'+info.Ordinal)), nil
		},
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { return nil },
		start:     func(context.Context, ToolUseStartInfo) error { return nil },
		finish: func(_ context.Context, info ToolUseFinishInfo) error {
			mu.Lock()
			finishes = append(finishes, info)
			mu.Unlock()
			if info.ToolUseID == "tlu_2" {
				close(secondFinished)
			}
			return nil
		},
	}
	makeTool := func(name, output string, waitForSecond bool) tool.Tool {
		return tool.NewFuncTool(name, name, tool.Definition{Name: name, InputSchema: tool.InputSchema{Type: "object"}}, func(_ context.Context, _ tool.Invocation) (tool.Result, error) {
			entered <- name
			<-release
			if waitForSecond {
				<-secondFinished
			}
			return tool.Result{Text: output}, nil
		})
	}
	type runOutput struct {
		result *Result
		err    error
	}
	done := make(chan runOutput, 1)
	go func() {
		result, err := Run(context.Background(), Config{
			Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{
				models.ToolCallPart{CallID: "same_call_id", Name: "first", Input: map[string]any{}},
				models.ToolCallPart{CallID: "same_call_id", Name: "second", Input: map[string]any{}},
			}}},
			Model: testModel, Tools: []tool.Tool{makeTool("first", "first output", true), makeTool("second", "second output", false)},
			MaxSteps: 1, ToolExecution: ToolExecutionParallel, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle,
		})
		done <- runOutput{result, err}
	}()
	<-entered
	<-entered
	close(release)
	out := <-done
	if out.err != nil {
		t.Fatal(out.err)
	}
	if len(proposalOrdinals) != 2 || proposalOrdinals[0] != 1 || proposalOrdinals[1] != 2 {
		t.Fatalf("proposal ordinals = %#v", proposalOrdinals)
	}
	if len(out.result.Turns) != 1 || len(out.result.Turns[0].Results) != 2 {
		t.Fatalf("turns = %#v", out.result.Turns)
	}
	results := out.result.Turns[0].Results
	if results[0].ToolUseID != "tlu_1" || results[0].Output != "first output" || results[1].ToolUseID != "tlu_2" || results[1].Output != "second output" {
		t.Fatalf("results = %#v", results)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(finishes) != 2 || finishes[0].ToolUseID != "tlu_2" || finishes[1].ToolUseID != "tlu_1" {
		t.Fatalf("finishes = %#v", finishes)
	}
	parts := out.result.Messages[0].Content
	first := parts[0].(models.ToolPart)
	second := parts[1].(models.ToolPart)
	if first.ID != "part_1" || first.ToolUseID != "tlu_1" || first.Output != "first output" || second.ID != "part_2" || second.ToolUseID != "tlu_2" || second.Output != "second output" {
		t.Fatalf("parts = %#v", parts)
	}
}

func TestBeforeToolCallFailureSettlesWithoutAuthorizationOrStart(t *testing.T) {
	var executions, authorizes, starts int
	var finished ToolUseFinishInfo
	lifecycle := toolUseLifecycleFuncs{
		propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "tlu_1", nil },
		authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorizes++; return nil },
		start:     func(context.Context, ToolUseStartInfo) error { starts++; return nil },
		finish:    func(_ context.Context, info ToolUseFinishInfo) error { finished = info; return nil },
	}
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executions++
		return tool.Result{}, nil
	})
	_, err := Run(context.Background(), Config{
		Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}},
		Model:  testModel, Tools: []tool.Tool{toolSpy}, MessageCheckpoint: identifiedCheckpoint(), ToolUseLifecycle: lifecycle,
		Hooks: Hooks{BeforeToolCall: func(context.Context, ToolCall) (map[string]any, error) { return nil, errors.New("blocked") }},
	})
	if err == nil || executions != 0 || authorizes != 0 || starts != 0 {
		t.Fatalf("err=%v executions=%d authorizes=%d starts=%d", err, executions, authorizes, starts)
	}
	if finished.Status != ToolUseStatusFailed || !finished.AuthorizedAt.IsZero() || !finished.StartedAt.IsZero() {
		t.Fatalf("finish = %#v", finished)
	}
}

type toolUseLifecycleFuncs struct {
	propose   func(context.Context, ToolUseProposeInfo) (string, error)
	authorize func(context.Context, ToolUseAuthorizeInfo) error
	start     func(context.Context, ToolUseStartInfo) error
	finish    func(context.Context, ToolUseFinishInfo) error
}

func (f toolUseLifecycleFuncs) Propose(ctx context.Context, info ToolUseProposeInfo) (string, error) {
	return f.propose(ctx, info)
}
func (f toolUseLifecycleFuncs) Authorize(ctx context.Context, info ToolUseAuthorizeInfo) error {
	return f.authorize(ctx, info)
}
func (f toolUseLifecycleFuncs) Start(ctx context.Context, info ToolUseStartInfo) error {
	return f.start(ctx, info)
}
func (f toolUseLifecycleFuncs) Finish(ctx context.Context, info ToolUseFinishInfo) error {
	return f.finish(ctx, info)
}
