package loop_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/loop/looptest"
	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
)

// TestLoopRespectsMaxSteps answers: Does the loop terminate at Config.MaxSteps even when the model would keep emitting tool calls forever?
func TestLoopRespectsMaxSteps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	script := make([]looptest.ScriptedReply, 10)
	for i := range script {
		script[i] = looptest.ReplyWithTool("noop", "{}")
	}
	model := looptest.NewRecordingModel(script...)
	sink := looptest.NewRecordingSink()

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	cfg := newConfig(t, model, sink, loop.Hooks{})
	cfg.MaxSteps = 3
	cfg.Tools = []tool.Tool{noop}

	res, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StopReason != loop.StopReasonMaxSteps {
		t.Fatalf("expected StopReasonMaxSteps, got %q", res.StopReason)
	}
	reqs := model.Requests()
	if len(reqs) != 3 {
		t.Fatalf("expected 3 model requests, got %d", len(reqs))
	}
}

// TestBeforeRunSeedsInitialMessages answers: Does Hooks.BeforeRun correctly seed the initial message history?
func TestBeforeRunSeedsInitialMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("ack"))
	sink := looptest.NewRecordingSink()

	seeded := []wingmodels.Message{
		wingmodels.NewUserText("seed-user"),
		wingmodels.NewAssistantText("seed-assistant"),
	}

	hooks := loop.Hooks{
		BeforeRun: func(ctx context.Context, current []wingmodels.Message) ([]wingmodels.Message, error) {
			return append([]wingmodels.Message(nil), seeded...), nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 model request, got %d", len(reqs))
	}
	msgs := reqs[0].Messages
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in first request, got %d", len(msgs))
	}
	if msgs[0].Role != wingmodels.RoleUser || !hasText(msgs[0], "seed-user") {
		t.Errorf("first message mismatch: role=%q", msgs[0].Role)
	}
	if msgs[1].Role != wingmodels.RoleAssistant || !hasText(msgs[1], "seed-assistant") {
		t.Errorf("second message mismatch: role=%q", msgs[1].Role)
	}
}

// TestBeforeRunAndConfigMessagesMutuallyExclusive answers: Does the loop reject a config that sets BOTH Hooks.BeforeRun AND a non-empty Config.Messages?
func TestBeforeRunAndConfigMessagesMutuallyExclusive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel()
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		BeforeRun: func(ctx context.Context, current []wingmodels.Message) ([]wingmodels.Message, error) {
			return []wingmodels.Message{wingmodels.NewUserText("x")}, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Messages = []wingmodels.Message{wingmodels.NewUserText("existing")}

	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error when both BeforeRun and Messages are set, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "BeforeRun") || !strings.Contains(msg, "Messages") {
		t.Errorf("expected error to mention BeforeRun and Messages, got: %v", err)
	}
	reqs := model.Requests()
	if len(reqs) != 0 {
		t.Errorf("expected 0 model requests, got %d", len(reqs))
	}
}

// TestBeforeStepMutationPersistsAcrossTurns answers: Does a mutation made by Hooks.BeforeStep persist to the next turn's running history?
func TestBeforeStepMutationPersistsAcrossTurns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", "{}"),
		looptest.Reply("done"),
	)
	sink := looptest.NewRecordingSink()

	marker := wingmodels.NewUserText("synthetic-marker")

	hooks := loop.Hooks{
		BeforeStep: func(ctx context.Context, info loop.BeforeStepInfo) ([]wingmodels.Message, error) {
			if info.Step == 1 {
				out := make([]wingmodels.Message, 0, len(info.Messages)+1)
				out = append(out, marker)
				out = append(out, info.Messages...)
				return out, nil
			}
			return info.Messages, nil
		},
	}

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 model requests, got %d", len(reqs))
	}

	msgs := reqs[1].Messages
	if len(msgs) == 0 {
		t.Fatalf("expected messages in second request")
	}
	if !hasText(msgs[0], "synthetic-marker") {
		t.Errorf("expected marker as first message in second request, got role=%q", msgs[0].Role)
	}
}

// TestTransformContextDoesNotPersist answers: Does a mutation made by Hooks.TransformContext apply ONLY to the current turn's wire request and NOT persist to the next turn?
func TestTransformContextDoesNotPersist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", "{}"),
		looptest.Reply("done"),
	)
	sink := looptest.NewRecordingSink()

	marker := wingmodels.NewUserText("transform-marker")

	hooks := loop.Hooks{
		TransformContext: func(ctx context.Context, info loop.TransformContextInfo) ([]wingmodels.Message, error) {
			if info.Step == 1 {
				out := make([]wingmodels.Message, 0, len(info.Messages)+1)
				out = append(out, marker)
				out = append(out, info.Messages...)
				return out, nil
			}
			return info.Messages, nil
		},
	}

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 model requests, got %d", len(reqs))
	}

	if !containsMessageWithText(reqs[0].Messages, "transform-marker") {
		t.Errorf("turn 1 request missing marker")
	}
	if containsMessageWithText(reqs[1].Messages, "transform-marker") {
		t.Errorf("turn 2 request unexpectedly contains marker")
	}
}

// TestBeforeToolCallSkipSynthesizesResult answers: When Hooks.BeforeToolCall returns loop.ErrSkipTool, does the loop synthesize a tool result and continue without invoking the tool?
func TestBeforeToolCallSkipSynthesizesResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var execCount int
	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		execCount++
		return "REAL_RESULT", nil
	})

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", "{}"),
		looptest.Reply("done"),
	)
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		BeforeToolCall: func(ctx context.Context, call loop.ToolCall) (map[string]any, error) {
			return nil, loop.ErrSkipTool
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if execCount != 0 {
		t.Errorf("expected tool execution count 0, got %d", execCount)
	}

	reqs := model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 model requests, got %d", len(reqs))
	}

	var found bool
	for _, m := range reqs[1].Messages {
		if m.Role != wingmodels.RoleTool {
			continue
		}
		for _, p := range m.Content {
			trp, ok := p.(wingmodels.ToolResultPart)
			if !ok {
				continue
			}
			found = true
			if len(trp.Output) != 1 {
				t.Errorf("expected 1 output part, got %d", len(trp.Output))
				continue
			}
			tp, ok := trp.Output[0].(wingmodels.TextPart)
			if !ok {
				t.Errorf("expected TextPart, got %T", trp.Output[0])
				continue
			}
			if tp.Text != "skip tool" {
				t.Errorf("expected synthetic result 'skip tool', got %q", tp.Text)
			}
			if !trp.IsError {
				t.Errorf("expected IsError=true on synthetic result")
			}
		}
	}
	if !found {
		t.Errorf("expected tool result message in second request")
	}
}

// TestAfterToolCallRewriteEchoedToModel answers: Does a rewritten result from Hooks.AfterToolCall actually appear in the next turn's wire request?
func TestAfterToolCallRewriteEchoedToModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ORIGINAL", nil
	})

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", "{}"),
		looptest.Reply("done"),
	)
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		AfterToolCall: func(ctx context.Context, call loop.ToolCall, result string, isError bool) (string, error) {
			if strings.Contains(result, "ORIGINAL") {
				return "REWRITTEN", nil
			}
			return result, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 model requests, got %d", len(reqs))
	}

	var found bool
	for _, m := range reqs[1].Messages {
		if m.Role != wingmodels.RoleTool {
			continue
		}
		for _, p := range m.Content {
			trp, ok := p.(wingmodels.ToolResultPart)
			if !ok {
				continue
			}
			found = true
			if len(trp.Output) != 1 {
				t.Errorf("expected 1 output part, got %d", len(trp.Output))
				continue
			}
			tp, ok := trp.Output[0].(wingmodels.TextPart)
			if !ok {
				t.Errorf("expected TextPart, got %T", trp.Output[0])
				continue
			}
			if tp.Text != "REWRITTEN" {
				t.Errorf("expected rewritten result 'REWRITTEN', got %q", tp.Text)
			}
		}
	}
	if !found {
		t.Errorf("expected tool result message in second request")
	}
}
