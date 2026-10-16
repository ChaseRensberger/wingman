package session

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

func TestSameLengthHistoryTransformAdoptsAndPersistsSyntheticMessage(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			if err := data.CreateSession(&store.Session{ID: "ses_transform"}); err != nil {
				t.Fatal(err)
			}
			client := &requestCaptureClient{}
			transformed := false
			sess := New(
				WithID("ses_transform"), WithStore(data), WithClient(client),
				WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
				WithTransformHistory(func(_ context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
					if transformed {
						return nil, nil
					}
					transformed = true
					message := models.Message{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "replacement"}}}
					info.Sink.OnEvent(run.MessageEvent{Message: message})
					return []models.Message{message}, nil
				}),
			)
			t.Cleanup(func() { _ = sess.Close(context.Background()) })
			if _, err := sess.Run(t.Context(), "original"); err != nil {
				t.Fatal(err)
			}
			if len(client.request.Messages) != 1 || textOf(client.request.Messages[0]) != "replacement" {
				t.Fatalf("request history = %#v", client.request.Messages)
			}
			if history := sess.History(); len(history) != 2 || textOf(history[0]) != "replacement" {
				t.Fatalf("session history = %#v", history)
			}
			if _, err := sess.Run(t.Context(), "continue"); err != nil {
				t.Fatal(err)
			}
			if len(client.request.Messages) != 3 || textOf(client.request.Messages[0]) != "replacement" {
				t.Fatalf("next run history = %#v", client.request.Messages)
			}
			hydrated := New(WithID("ses_transform"), WithStore(data))
			t.Cleanup(func() { _ = hydrated.Close(context.Background()) })
			if err := hydrated.hydrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			history := hydrated.History()
			// Transforming active history must not delete the original durable input.
			if len(history) != 5 || textOf(history[0]) != "original" || textOf(history[1]) != "replacement" || textOf(history[2]) != "done" || textOf(history[3]) != "continue" || textOf(history[4]) != "done" {
				t.Fatalf("stored history = %#v", history)
			}
		})
	}
}
