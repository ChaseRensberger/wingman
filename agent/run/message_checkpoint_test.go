package run

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

type checkpointFunc func(context.Context, MessageCheckpointInfo) (models.Message, error)

func (f checkpointFunc) Save(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
	return f(ctx, info)
}

func identifiedCheckpoint() MessageCheckpoint {
	return checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
		message := info.Message
		if message.ID == "" {
			message.ID = "message_1"
		}
		for i, part := range message.Content {
			if models.PartID(part) == "" {
				message.Content[i] = models.WithPartID(part, "part_"+string(rune('1'+i)))
			}
		}
		return message, nil
	})
}

func TestMessageCheckpointPrecedesModelStartAndDispatch(t *testing.T) {
	var order []string
	client := &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		order = append(order, "stream")
		return completedStream(models.Message{Role: models.RoleAssistant}), nil
	}}
	_, err := Run(context.Background(), Config{Client: client, Model: testModel,
		MessageCheckpoint: checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
			order = append(order, "checkpoint")
			if len(order) == 1 && (info.Message.State != models.MessageStateInProgress || info.Message.Revision != 1) {
				t.Fatalf("initial = %#v", info.Message)
			}
			info.Message.ID = "message_1"
			return info.Message, nil
		}),
		ModelCallLifecycle: modelCallLifecycleFuncs{start: func(_ context.Context, info ModelCallStartInfo) (string, error) {
			order = append(order, "start")
			if info.MessageID != "message_1" {
				t.Fatalf("MessageID = %q", info.MessageID)
			}
			return "call_1", nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) < 3 || order[0] != "checkpoint" || order[1] != "start" || order[2] != "stream" {
		t.Fatalf("order = %#v", order)
	}
}

func TestMessageCheckpointStreamIdentityAndStablePartID(t *testing.T) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	go func() {
		stream.Push(models.TextStartPart{ID: "provider_text"})
		stream.Push(models.TextDeltaPart{ID: "provider_text", Delta: "hello"})
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "hello"}}}, nil)
	}()
	var events []StreamPartEvent
	var mu sync.Mutex
	result, err := Run(context.Background(), Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		return stream, nil
	}}, Model: testModel, MessageCheckpoint: identifiedCheckpoint(), Sink: SinkFunc(func(event Event) {
		if e, ok := event.(StreamPartEvent); ok {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	message := result.Messages[0]
	if message.ID != "message_1" || message.Revision < 4 || models.PartID(message.Content[0]) != "part_1" {
		t.Fatalf("message = %#v", message)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) < 2 || events[0].MessageID != "message_1" || events[0].PartID != "part_1" || events[1].PartID != "part_1" || events[1].Revision <= events[0].Revision {
		t.Fatalf("events = %#v", events)
	}
}

func TestMessageCheckpointRetainsPartialAssistantOnStreamError(t *testing.T) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	go func() {
		stream.Push(models.TextStartPart{ID: "provider_text"})
		stream.Push(models.TextDeltaPart{ID: "provider_text", Delta: "partial"})
		stream.Close(nil, errors.New("connection lost"))
	}()
	result, err := Run(context.Background(), Config{
		Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return stream, nil
		}},
		Model:             testModel,
		MessageCheckpoint: identifiedCheckpoint(),
	})
	if err == nil {
		t.Fatal("Run succeeded, want stream error")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %#v, want one partial assistant", result.Messages)
	}
	message := result.Messages[0]
	text, ok := message.Content[0].(models.TextPart)
	if !ok || text.Text != "partial" || text.ID != "part_1" || message.ID != "message_1" || message.State != models.MessageStateFailed {
		t.Fatalf("partial assistant = %#v", message)
	}
}

func TestMessageCheckpointPreservesReasoningAndRawToolInputIdentity(t *testing.T) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	go func() {
		stream.Push(models.ReasoningStartPart{ID: "provider_reasoning"})
		stream.Push(models.ReasoningDeltaPart{ID: "provider_reasoning", Delta: "think"})
		stream.Push(models.ToolInputStartPart{ID: "call_1", ToolName: "unknown"})
		stream.Push(models.ToolInputDeltaPart{ID: "call_1", Delta: `{"x":1}`})
		stream.Push(models.ToolCallPart_{ID: "call_1", ToolName: "unknown", Input: map[string]any{"x": float64(1)}})
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{
			models.ReasoningPart{Reasoning: "think"},
			models.ToolCallPart{CallID: "call_1", Name: "unknown", Input: map[string]any{"x": float64(1)}},
		}}, nil)
	}()
	var events []StreamPartEvent
	result, err := Run(context.Background(), Config{
		Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			return stream, nil
		}},
		Model:             testModel,
		MaxSteps:          1,
		MessageCheckpoint: identifiedCheckpoint(),
		Sink: SinkFunc(func(event Event) {
			if streamEvent, ok := event.(StreamPartEvent); ok {
				events = append(events, streamEvent)
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := result.Messages[0]
	if len(message.Content) != 2 {
		t.Fatalf("content = %#v", message.Content)
	}
	reasoning, reasoningOK := message.Content[0].(models.ReasoningPart)
	toolPart, toolOK := message.Content[1].(models.ToolPart)
	if !reasoningOK || reasoning.ID != "part_1" || reasoning.Reasoning != "think" {
		t.Fatalf("reasoning = %#v", message.Content[0])
	}
	if !toolOK || toolPart.ID != "part_2" || toolPart.InputRaw != `{"x":1}` || toolPart.CallID != "call_1" {
		t.Fatalf("tool = %#v", message.Content[1])
	}
	for _, event := range events {
		switch event.Part.(type) {
		case models.ReasoningStartPart, models.ReasoningDeltaPart:
			if event.PartID != reasoning.ID {
				t.Fatalf("reasoning event identity = %#v", event)
			}
		case models.ToolInputStartPart, models.ToolInputDeltaPart, models.ToolCallPart_:
			if event.PartID != toolPart.ID {
				t.Fatalf("tool event identity = %#v", event)
			}
		}
	}
}

func TestCheckpointFailureBeforeToolsPreventsExecution(t *testing.T) {
	called := false
	checkpointCalls := 0
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) { called = true; return tool.Result{}, nil })
	_, err := Run(context.Background(), Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}}, Model: testModel, Tools: []tool.Tool{toolSpy}, MessageCheckpoint: checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
		checkpointCalls++
		if checkpointCalls == 3 {
			return models.Message{}, errors.New("pending save failed")
		}
		return info.Message, nil
	})})
	if err == nil || called {
		t.Fatalf("err = %v, tool called = %v", err, called)
	}
}

func TestPendingToolSnapshotPrecedesExecutionAndCompletionIsCheckpointed(t *testing.T) {
	pendingSaved := false
	completedRevision := int64(0)
	called := false
	toolSpy := tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		if !pendingSaved {
			t.Fatal("tool ran before pending snapshot")
		}
		called = true
		return tool.Result{}, nil
	})
	result, err := Run(context.Background(), Config{Client: lifecycleClient{message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{}}}}}, Model: testModel, Tools: []tool.Tool{toolSpy}, MaxSteps: 1, MessageCheckpoint: checkpointFunc(func(_ context.Context, info MessageCheckpointInfo) (models.Message, error) {
		if len(info.Message.Content) == 1 {
			if part, ok := info.Message.Content[0].(models.ToolPart); ok && part.State == models.ToolStatePending {
				pendingSaved = true
			}
		}
		if info.Message.State == models.MessageStateCompleted {
			completedRevision = info.Message.Revision
		}
		return info.Message, nil
	})})
	if err != nil || !called || completedRevision == 0 || result.Messages[0].Revision != completedRevision {
		t.Fatalf("err=%v called=%v completed revision=%d result=%#v", err, called, completedRevision, result)
	}
}
