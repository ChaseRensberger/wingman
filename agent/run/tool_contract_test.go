package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestRunRejectsDuplicateToolsBeforeModelDispatch(t *testing.T) {
	toolFor := func() tool.Tool {
		return tool.NewFuncTool("duplicate", "", tool.Definition{Name: "duplicate", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
			return tool.Result{}, nil
		})
	}
	_, err := Run(context.Background(), Config{
		Client: noDispatchClient{},
		Model:  models.ModelRef{Provider: "test", ID: "model"},
		Tools:  []tool.Tool{toolFor(), toolFor()},
	})
	if !errors.Is(err, tool.ErrDuplicateTool) {
		t.Fatalf("Run() error = %v, want duplicate tool error", err)
	}
}

func TestExecuteOneMarksInvalidStructuredResult(t *testing.T) {
	structured := map[string]any{"count": "not a number"}
	call := ToolCall{
		ID: "call_1", Name: "structured", Args: map[string]any{},
		Tool: tool.NewFuncTool("structured", "", tool.Definition{
			Name:         "structured",
			InputSchema:  tool.InputSchema{Type: "object"},
			OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"count": map[string]any{"type": "number"}}, "required": []string{"count"}},
		}, func(context.Context, tool.Invocation) (tool.Result, error) {
			return tool.Result{Text: "partial", Metadata: map[string]any{"source": "test"}, Structured: structured}, nil
		}),
	}
	r := &runner{eventCh: make(chan Event, 2)}
	result, err := r.executeOne(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Error == "" || result.Output != "partial" || result.Metadata["source"] != "test" || result.Structured.(map[string]any)["count"] != "not a number" {
		t.Fatalf("result = %#v", result)
	}
	if result.Status != ToolUseStatusFailed || result.ErrorType != "result_validation" {
		t.Fatalf("status/error type = %q/%q, want failed/result_validation", result.Status, result.ErrorType)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"structured":{"count":"not a number"}`) {
		t.Fatalf("ToolResult JSON = %s, %v", encoded, err)
	}
}

func TestExecuteOneRejectsMissingStructuredResult(t *testing.T) {
	call := ToolCall{
		ID: "call_1", Name: "structured", Args: map[string]any{},
		Tool: tool.NewFuncTool("structured", "", tool.Definition{
			Name:         "structured",
			InputSchema:  tool.InputSchema{Type: "object"},
			OutputSchema: map[string]any{"type": "object"},
		}, func(context.Context, tool.Invocation) (tool.Result, error) {
			return tool.Result{Text: "partial", Metadata: map[string]any{"source": "test"}}, nil
		}),
	}
	result, err := (&runner{eventCh: make(chan Event, 2)}).executeOne(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Status != ToolUseStatusFailed || result.ErrorType != "result_validation" || !strings.Contains(result.Error, "structured result is required") || result.Output != "partial" || result.Metadata["source"] != "test" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteOnePassesInvocationIdentity(t *testing.T) {
	var got tool.Invocation
	call := ToolCall{
		ID: "call_1", ToolUseID: "tlu_1", MessageID: "msg_1", PartID: "prt_1", ModelCallID: "mdl_1",
		Name: "identity", Args: map[string]any{}, Tool: tool.NewFuncTool("identity", "", tool.Definition{Name: "identity", InputSchema: tool.InputSchema{Type: "object"}}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
			got = inv
			return tool.Result{}, nil
		}),
	}
	r := &runner{cfg: Config{SessionID: "ses_1", RunID: "run_1", AgentID: "agt_1"}, eventCh: make(chan Event, 2)}
	if _, err := r.executeOne(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "ses_1" || got.RunID != "run_1" || got.AgentID != "agt_1" || got.ToolUseID != "tlu_1" || got.CallID != "call_1" || got.MessageID != "msg_1" || got.PartID != "prt_1" || got.ModelCallID != "mdl_1" {
		t.Fatalf("invocation identity = %#v", got)
	}
}

type noDispatchClient struct{}

func (noDispatchClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	panic("model dispatched")
}
func (noDispatchClient) Generate(context.Context, models.Request) (*models.Message, error) {
	panic("model dispatched")
}
func (noDispatchClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	panic("model dispatched")
}
