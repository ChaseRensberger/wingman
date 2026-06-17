package run_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/run/runtest"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestRunValidatesToolInputBeforeExecution(t *testing.T) {
	executed := false
	lookup := tool.NewFuncTool("lookup", "Lookup a query", tool.Definition{
		Name:        "lookup",
		Description: "Lookup a query",
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.Property{
				"query": {Type: "string"},
			},
			Required: []string{"query"},
		},
	}, func(context.Context, map[string]any, string) (tool.Result, error) {
		executed = true
		return tool.Result{Text: "should not run"}, nil
	})

	model := runtest.NewRecordingModel(
		runtest.ReplyWithToolCalls(runtest.ToolCall{Name: "lookup", Args: map[string]any{"query": 42}}),
		runtest.Reply("done"),
	)
	sink := runtest.NewRecordingSink()
	result, err := run.Run(context.Background(), run.Config{
		Client: model,
		Model:  models.ModelRef{Provider: "runtest", ID: "model"},
		Tools:  []tool.Tool{lookup},
		Sink:   sink,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if executed {
		t.Fatal("tool executed despite invalid input")
	}
	if result.StopReason != run.StopReasonEndTurn {
		t.Fatalf("stop = %q, want end turn", result.StopReason)
	}
	ends := sink.ToolEnds()
	if len(ends) != 1 {
		t.Fatalf("tool end count = %d, want 1", len(ends))
	}
	if !ends[0].Result.IsError || !strings.Contains(ends[0].Result.Output, "input validation error") {
		t.Fatalf("result = %#v", ends[0].Result)
	}
}
