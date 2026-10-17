package run

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestContextOverflowRecoveryBounds(t *testing.T) {
	for _, tc := range []struct {
		name                                              string
		partial, unchanged, twice, persistence, recovered bool
		maxSteps, wantCalls, wantRecovery                 int
	}{
		{name: "recover", wantCalls: 2, wantRecovery: 1},
		{name: "no-change", unchanged: true, wantCalls: 1, wantRecovery: 1},
		{name: "partial", partial: true, wantCalls: 1},
		{name: "second-overflow", twice: true, wantCalls: 2, wantRecovery: 1},
		{name: "persistence-error", persistence: true, wantCalls: 1},
		{name: "step-limit", maxSteps: 1, wantCalls: 1},
		{name: "restarted", recovered: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, recoveries := 0, 0
			overflow := models.ClassifyProviderFailure("test", 400, `{"error":{"code":"context_length_exceeded"}}`)
			cfg := Config{Model: testModel, MaxSteps: tc.maxSteps, Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
				calls++
				if calls == 1 || tc.twice {
					if !tc.partial {
						return nil, overflow
					}
					stream := models.NewEventStream[models.StreamPart, *models.Message](2)
					stream.Push(models.TextStartPart{ID: "text"})
					stream.Push(models.TextDeltaPart{ID: "text", Delta: "partial"})
					stream.Close(nil, overflow)
					return stream, nil
				}
				stream := models.NewEventStream[models.StreamPart, *models.Message](0)
				stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}, nil)
				return stream, nil
			}}, Hooks: Hooks{TransformHistory: func(_ context.Context, info TransformHistoryInfo) ([]models.Message, error) {
				if !info.ContextOverflow {
					return nil, nil
				}
				recoveries++
				if tc.unchanged {
					return nil, nil
				}
				return []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "summary"}}}}, nil
			}}}
			if tc.persistence {
				cfg.MessageCheckpoint = checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
					if info.Message.State == models.MessageStateFailed {
						return models.Message{}, errors.New("save failed")
					}
					return identifiedCheckpoint().Save(ctx, info)
				})
			}
			if tc.recovered {
				cfg.Recovery = &Recovery{OverflowRecovered: true}
			}
			_, err := Run(t.Context(), cfg)
			if (err == nil) != (tc.name == "recover") || calls != tc.wantCalls || recoveries != tc.wantRecovery {
				t.Fatalf("calls=%d recoveries=%d err=%v", calls, recoveries, err)
			}
		})
	}
}

func TestRecoveredOverflowRequiresHistoryTransform(t *testing.T) {
	calls := 0
	_, err := Run(t.Context(), Config{Model: testModel, Recovery: &Recovery{OverflowRecovered: true, ContextOverflow: true}, Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		calls++
		return nil, errors.New("unexpected dispatch")
	}}})
	if err == nil || calls != 0 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
