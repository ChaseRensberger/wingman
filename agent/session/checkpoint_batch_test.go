package session

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

type checkpointMeasureStore struct {
	store.Store
	mu           sync.Mutex
	saves, bytes int
}

func (s *checkpointMeasureStore) SaveMessage(ctx context.Context, message store.StoredMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.saves++
	s.bytes += len(data)
	s.mu.Unlock()
	return s.Store.SaveMessage(ctx, message)
}

type checkpointBurstClient struct{ requestCaptureClient }

func (*checkpointBurstClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	go func() {
		stream.Push(models.TextStartPart{ID: "text"})
		for i := 0; i < 1024; i++ {
			stream.Push(models.TextDeltaPart{ID: "text", Delta: strings.Repeat("x", 32)})
		}
		stream.Push(models.TextEndPart{ID: "text"})
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: strings.Repeat("x", 32768)}}}, nil)
	}()
	return stream, nil
}

func TestStreamingCheckpointWriteAmplification(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := &checkpointMeasureStore{Store: historyStore(t, backend)}
			sess := New(WithID("ses_history"), WithStore(data), WithClient(&checkpointBurstClient{}), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}))
			t.Cleanup(func() { _ = sess.Close(context.Background()) })
			if _, err := sess.Run(t.Context(), "go"); err != nil {
				t.Fatal(err)
			}
			events, err := data.Store.(store.AggregateEventReader).ListAggregateEvents(t.Context(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_history"}, 0, 10000)
			if err != nil {
				t.Fatal(err)
			}
			count, bytes := 0, 0
			for _, event := range events {
				if event.Type == store.EventSessionMessageSaved {
					count++
					bytes += len(event.Data)
				}
			}
			t.Logf("32 KiB / 1024 deltas: save calls=%d snapshot bytes=%d durable snapshots=%d event bytes=%d", data.saves, data.bytes, count, bytes)
			if data.saves > 80 || data.bytes > 2*1024*1024 || bytes > 2*1024*1024 {
				t.Error("excessive full-message checkpoint writes")
			}
			if err := data.Store.(interface {
				RebuildSessionProjections(context.Context, string) error
			}).RebuildSessionProjections(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			reopened := New(WithID("ses_history"), WithStore(data))
			t.Cleanup(func() { _ = reopened.Close(context.Background()) })
			if err := reopened.hydrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			history := reopened.History()
			if len(history) != 2 || history[1].Content[0].(models.TextPart).Text != strings.Repeat("x", 32768) {
				t.Fatal("replay lost final output")
			}
		})
	}
}
