package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
)

func TestSystemTransformsComposeAndStopOnError(t *testing.T) {
	wantErr := errors.New("context unavailable")
	for _, fail := range []bool{false, true} {
		var registry *Registry
		secondCalled := false
		first := testPlugin{name: "first", activate: func(r *Registry) (Cleanup, error) {
			registry = r
			return nil, r.RegisterTransformSystem(func(_ context.Context, info run.TransformSystemInfo) (string, error) {
				if fail {
					return "", wantErr
				}
				return info.System + " first", nil
			})
		}}
		second := testPlugin{name: "second", activate: func(r *Registry) (Cleanup, error) {
			return nil, r.RegisterTransformSystem(func(_ context.Context, info run.TransformSystemInfo) (string, error) {
				secondCalled = true
				if info.Model.ID != "current" || info.WorkDir != "/execution" {
					t.Fatalf("lost call metadata: %#v", info)
				}
				return info.System + " second", nil
			})
		}}
		generation, err := ActivateAll(first, second)
		if err != nil {
			t.Fatal(err)
		}
		text, err := generation.Runtime().Hooks.TransformSystem(t.Context(), run.TransformSystemInfo{System: "base", Model: models.ModelRef{ID: "current"}, WorkDir: "/execution"})
		if fail {
			if !errors.Is(err, wantErr) || secondCalled {
				t.Fatalf("err=%v second=%v", err, secondCalled)
			}
		} else if err != nil || text != "base first second" {
			t.Fatalf("text=%q err=%v", text, err)
		}
		if err := registry.RegisterTransformSystem(nil); err == nil {
			t.Fatal("registered after activation")
		}
		if err := generation.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}
