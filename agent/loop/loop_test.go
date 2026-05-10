package loop_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/tool"
	"github.com/chaserensberger/wingman/models"
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

	seeded := []models.Message{
		models.NewUserText("seed-user"),
		models.NewAssistantText("seed-assistant"),
	}

	hooks := loop.Hooks{
		BeforeRun: func(ctx context.Context, current []models.Message) ([]models.Message, error) {
			return append([]models.Message(nil), seeded...), nil
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
	if msgs[0].Role != models.RoleUser || !hasText(msgs[0], "seed-user") {
		t.Errorf("first message mismatch: role=%q", msgs[0].Role)
	}
	if msgs[1].Role != models.RoleAssistant || !hasText(msgs[1], "seed-assistant") {
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
		BeforeRun: func(ctx context.Context, current []models.Message) ([]models.Message, error) {
			return []models.Message{models.NewUserText("x")}, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Messages = []models.Message{models.NewUserText("existing")}

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

// TestTransformHistoryMutationPersistsAcrossTurns answers: Does a mutation made by Hooks.TransformHistory persist to the next turn's running history?
func TestTransformHistoryMutationPersistsAcrossTurns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("noop", "{}"),
		looptest.Reply("done"),
	)
	sink := looptest.NewRecordingSink()

	marker := models.NewUserText("synthetic-marker")

	hooks := loop.Hooks{
		TransformHistory: func(ctx context.Context, info loop.TransformHistoryInfo) ([]models.Message, error) {
			if info.Step == 1 {
				out := make([]models.Message, 0, len(info.Messages)+1)
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

	marker := models.NewUserText("transform-marker")

	hooks := loop.Hooks{
		TransformContext: func(ctx context.Context, info loop.TransformContextInfo) ([]models.Message, error) {
			if info.Step == 1 {
				out := make([]models.Message, 0, len(info.Messages)+1)
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
		if m.Role != models.RoleTool {
			continue
		}
		for _, p := range m.Content {
			trp, ok := p.(models.ToolResultPart)
			if !ok {
				continue
			}
			found = true
			if len(trp.Output) != 1 {
				t.Errorf("expected 1 output part, got %d", len(trp.Output))
				continue
			}
			tp, ok := trp.Output[0].(models.TextPart)
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
		if m.Role != models.RoleTool {
			continue
		}
		for _, p := range m.Content {
			trp, ok := p.(models.ToolResultPart)
			if !ok {
				continue
			}
			found = true
			if len(trp.Output) != 1 {
				t.Errorf("expected 1 output part, got %d", len(trp.Output))
				continue
			}
			tp, ok := trp.Output[0].(models.TextPart)
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

// TestAfterRunFiresOnSuccess answers: Does AfterRun fire on a successful run with a non-nil Result and nil Err?
func TestAfterRunFiresOnSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	var gotInfo loop.AfterRunInfo
	hooks := loop.Hooks{
		AfterRun: func(ctx context.Context, info loop.AfterRunInfo) error {
			gotInfo = info
			return nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	res, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil Result")
	}
	if gotInfo.Result.StopReason != loop.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn, got %q", gotInfo.Result.StopReason)
	}
	if gotInfo.Err != nil {
		t.Errorf("expected nil Err in AfterRunInfo, got %v", gotInfo.Err)
	}
}

// TestAfterRunFiresOnError answers: Does AfterRun fire on an error path with the run error visible in info.Err?
func TestAfterRunFiresOnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.ReplyError(errors.New("model exploded")))
	sink := looptest.NewRecordingSink()

	var gotInfo loop.AfterRunInfo
	hooks := loop.Hooks{
		AfterRun: func(ctx context.Context, info loop.AfterRunInfo) error {
			gotInfo = info
			return nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if gotInfo.Err == nil {
		t.Fatal("expected non-nil Err in AfterRunInfo")
	}
	if !strings.Contains(gotInfo.Err.Error(), "model exploded") {
		t.Errorf("expected error to contain 'model exploded', got %v", gotInfo.Err)
	}
	if gotInfo.Result.StopReason != loop.StopReasonError {
		t.Errorf("expected StopReasonError, got %q", gotInfo.Result.StopReason)
	}
}

// TestAfterRunErrorJoinsWithRunError answers: Is an AfterRun error joined with the existing run error?
func TestAfterRunErrorJoinsWithRunError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.ReplyError(errors.New("model exploded")))
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		AfterRun: func(ctx context.Context, info loop.AfterRunInfo) error {
			return errors.New("after run failed")
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model exploded") {
		t.Errorf("expected error to contain 'model exploded', got %v", err)
	}
	if !strings.Contains(err.Error(), "after run failed") {
		t.Errorf("expected error to contain 'after run failed', got %v", err)
	}
}

// TestTransformToolDefsRewriteReachesModel answers: Does a TransformToolDefs rewrite actually reach the model request?
func TestTransformToolDefsRewriteReachesModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	hooks := loop.Hooks{
		TransformToolDefs: func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			out := append([]models.ToolDef(nil), info.Tools...)
			if len(out) > 0 {
				out[0].Description = "rewritten description"
			}
			return out, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if len(reqs[0].Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(reqs[0].Tools))
	}
	if reqs[0].Tools[0].Description != "rewritten description" {
		t.Errorf("expected rewritten description, got %q", reqs[0].Tools[0].Description)
	}
}

// TestTransformToolDefsNilSendsNoTools answers: Does TransformToolDefs returning a nil slice send no tools to the model?
func TestTransformToolDefsNilSendsNoTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	noop := tool.NewFuncTool("noop", "does nothing", tool.Definition{
		Name:        "noop",
		Description: "A no-op tool for testing.",
		InputSchema: tool.InputSchema{Type: "object"},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "ok", nil
	})

	hooks := loop.Hooks{
		TransformToolDefs: func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			return nil, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	cfg.Tools = []tool.Tool{noop}

	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if len(reqs[0].Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(reqs[0].Tools))
	}
}

// TestTransformToolDefsErrorFailsRun answers: Does a TransformToolDefs error fail the run?
func TestTransformToolDefsErrorFailsRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		TransformToolDefs: func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			return nil, errors.New("tool defs transform failed")
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tool defs transform failed") {
		t.Errorf("expected error to contain 'tool defs transform failed', got %v", err)
	}
}

// TestTransformParamsRewriteReachesModel answers: Does a TransformParams rewrite actually reach the model request?
func TestTransformParamsRewriteReachesModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		TransformParams: func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			tokens := 512
			return loop.TransformParamsResult{Params: loop.SamplingParams{MaxOutputTokens: &tokens}}, nil
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	_, err := loop.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].MaxOutputTokens != 512 {
		t.Errorf("expected MaxOutputTokens=512, got %d", reqs[0].MaxOutputTokens)
	}
}

// TestTransformParamsErrorFailsRun answers: Does a TransformParams error fail the run?
func TestTransformParamsErrorFailsRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := looptest.NewRecordingModel(looptest.Reply("done"))
	sink := looptest.NewRecordingSink()

	hooks := loop.Hooks{
		TransformParams: func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			return loop.TransformParamsResult{}, errors.New("params transform failed")
		},
	}

	cfg := newConfig(t, model, sink, hooks)
	_, err := loop.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "params transform failed") {
		t.Errorf("expected error to contain 'params transform failed', got %v", err)
	}
}
