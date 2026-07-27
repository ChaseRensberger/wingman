package run

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
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
	if part.State != models.ToolStatePending || part.CallID != "call_1" {
		t.Fatalf("tool part = %#v, want pending call_1", part)
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
