package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/models"
)

// TestComposeAfterRunRunsAllAndJoinsErrors answers: Does composeAfterRun run every hook and join all errors?
func TestComposeAfterRunRunsAllAndJoinsErrors(t *testing.T) {
	var calls []string
	hooks := []loop.AfterRunHook{
		func(ctx context.Context, info loop.AfterRunInfo) error {
			calls = append(calls, "first")
			return errors.New("first error")
		},
		func(ctx context.Context, info loop.AfterRunInfo) error {
			calls = append(calls, "second")
			return errors.New("second error")
		},
	}

	composed := composeAfterRun(hooks)
	err := composed(context.Background(), loop.AfterRunInfo{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}
	if calls[0] != "first" || calls[1] != "second" {
		t.Errorf("unexpected call order: %v", calls)
	}
	if !strings.Contains(err.Error(), "first error") || !strings.Contains(err.Error(), "second error") {
		t.Errorf("expected both errors, got %v", err)
	}
}

// TestComposeTransformToolDefsChainsAndShortCircuits answers: Does composeTransformToolDefs chain in order and short-circuit on error?
func TestComposeTransformToolDefsChainsAndShortCircuits(t *testing.T) {
	var calls []string
	hooks := []loop.TransformToolDefsHook{
		func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			calls = append(calls, "first")
			out := append([]models.ToolDef(nil), info.Tools...)
			out = append(out, models.ToolDef{Name: "a"})
			return out, nil
		},
		func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			calls = append(calls, "second")
			out := append([]models.ToolDef(nil), info.Tools...)
			out = append(out, models.ToolDef{Name: "b"})
			return out, nil
		},
	}

	composed := composeTransformToolDefs(hooks)
	res, err := composed(context.Background(), loop.TransformToolDefsInfo{Tools: []models.ToolDef{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}
	if len(res) != 2 || res[0].Name != "a" || res[1].Name != "b" {
		t.Errorf("unexpected result: %v", res)
	}

	// Short-circuit test
	calls = nil
	failingHooks := []loop.TransformToolDefsHook{
		func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			calls = append(calls, "first")
			return nil, errors.New("boom")
		},
		func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
			calls = append(calls, "second")
			return nil, nil
		},
	}
	composed = composeTransformToolDefs(failingHooks)
	_, err = composed(context.Background(), loop.TransformToolDefsInfo{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(calls) != 1 || calls[0] != "first" {
		t.Errorf("expected only first hook to run, got %v", calls)
	}
}

// TestComposeTransformParamsChainsAndShortCircuits answers: Does composeTransformParams chain in order and short-circuit on error?
func TestComposeTransformParamsChainsAndShortCircuits(t *testing.T) {
	var calls []string
	hooks := []loop.TransformParamsHook{
		func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			calls = append(calls, "first")
			tokens := 100
			return loop.TransformParamsResult{Params: loop.SamplingParams{MaxOutputTokens: &tokens}}, nil
		},
		func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			calls = append(calls, "second")
			if info.Params.MaxOutputTokens == nil || *info.Params.MaxOutputTokens != 100 {
				return loop.TransformParamsResult{}, errors.New("did not receive previous output")
			}
			tokens := 200
			return loop.TransformParamsResult{Params: loop.SamplingParams{MaxOutputTokens: &tokens}}, nil
		},
	}

	composed := composeTransformParams(hooks)
	res, err := composed(context.Background(), loop.TransformParamsInfo{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}
	if res.Params.MaxOutputTokens == nil || *res.Params.MaxOutputTokens != 200 {
		t.Errorf("expected MaxOutputTokens=200, got %v", res.Params.MaxOutputTokens)
	}

	// Short-circuit test
	calls = nil
	failingHooks := []loop.TransformParamsHook{
		func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			calls = append(calls, "first")
			return loop.TransformParamsResult{}, errors.New("boom")
		},
		func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
			calls = append(calls, "second")
			return loop.TransformParamsResult{}, nil
		},
	}
	composed = composeTransformParams(failingHooks)
	_, err = composed(context.Background(), loop.TransformParamsInfo{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(calls) != 1 || calls[0] != "first" {
		t.Errorf("expected only first hook to run, got %v", calls)
	}
}
