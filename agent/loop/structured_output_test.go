package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

// TestStructuredOutputFinalMessageParsed answers: when an OutputSchema is
// active and the model returns valid JSON, is RunResult.StructuredOutput
// populated with the parsed value and a StructuredOutputEvent fired?
func TestStructuredOutputFinalMessageParsed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply(`{"name":"Alice"}`))
	model.SetInfo(models.ModelInfo{
		Provider:     "looptest",
		ID:           "recording",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.OutputSchema = &models.OutputSchema{
		Name: "contact",
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}

	res, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StructuredOutput == nil {
		t.Fatal("expected StructuredOutput")
	}
	if res.StructuredOutput["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", res.StructuredOutput["name"])
	}
	events := sink.StructuredOutputs()
	if len(events) != 1 {
		t.Fatalf("expected 1 StructuredOutputEvent, got %d", len(events))
	}
	if events[0].Schema != "contact" {
		t.Errorf("schema = %q, want contact", events[0].Schema)
	}
}

// TestStructuredOutputSchemaMismatchHardErrors answers: when the model
// returns JSON missing a required field, does the run return an error and
// leave StructuredOutput nil?
func TestStructuredOutputSchemaMismatchHardErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply(`{"email":"a@b.com"}`))
	model.SetInfo(models.ModelInfo{
		Provider:     "looptest",
		ID:           "recording",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.OutputSchema = &models.OutputSchema{
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}

	res, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for schema mismatch")
	}
	if res.StructuredOutput != nil {
		t.Errorf("expected nil StructuredOutput, got %v", res.StructuredOutput)
	}
}

// TestStructuredOutputUnaffectedWhenNoSchema answers: without an active
// schema, does the loop complete normally with nil StructuredOutput?
func TestStructuredOutputUnaffectedWhenNoSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply(`hello`))
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	res, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StructuredOutput != nil {
		t.Errorf("expected nil StructuredOutput, got %v", res.StructuredOutput)
	}
	if len(sink.StructuredOutputs()) != 0 {
		t.Errorf("expected no StructuredOutputEvent, got %d", len(sink.StructuredOutputs()))
	}
}

// TestStructuredOutputPerRequestOverridesAgent answers: when the loop
// Config carries an OutputSchema, does the model receive it on the wire?
func TestStructuredOutputPerRequestOverridesAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply(`{"x":1}`))
	model.SetInfo(models.ModelInfo{
		Provider:     "looptest",
		ID:           "recording",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.OutputSchema = &models.OutputSchema{
		Name: "schema-b",
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"x": map[string]any{"type": "integer"},
			},
			"required": []any{"x"},
		},
	}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].OutputSchema == nil {
		t.Fatal("expected OutputSchema in wire request")
	}
	if reqs[0].OutputSchema.Name != "schema-b" {
		t.Errorf("schema name = %q, want schema-b", reqs[0].OutputSchema.Name)
	}
}

// TestStructuredOutputCapabilityGate answers: when a schema is set but the
// model's StructuredOutput capability is false, does the run error before
// any wire call?
func TestStructuredOutputCapabilityGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply(`{}`))
	// Default Info has StructuredOutput=false.
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.OutputSchema = &models.OutputSchema{Schema: map[string]any{"type": "object"}}

	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for unsupported model")
	}
	if len(model.Requests()) != 0 {
		t.Errorf("expected 0 wire calls, got %d", len(model.Requests()))
	}
}

// TestStructuredOutputDuringToolCalls answers: when the model emits a tool
// call on turn 1 and text-only JSON on turn 2, does the tool call run
// normally and the final structured output parse correctly?
func TestStructuredOutputDuringToolCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", `{}`),
		looptest.Reply(`{"result":"done"}`),
	)
	model.SetInfo(models.ModelInfo{
		Provider:     "looptest",
		ID:           "recording",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})
	sink := looptest.NewRecordingSink()

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.Tools = []tool.Tool{noop}
	cfg.OutputSchema = &models.OutputSchema{
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"result": map[string]any{"type": "string"},
			},
			"required": []any{"result"},
		},
	}

	res, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StructuredOutput == nil {
		t.Fatal("expected StructuredOutput after tool-call turn")
	}
	if res.StructuredOutput["result"] != "done" {
		t.Errorf("result = %v, want done", res.StructuredOutput["result"])
	}
	reqs := model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
}
