package session

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugins/compaction"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func historyStore(t testing.TB, backend string) store.Store {
	t.Helper()
	var data store.Store = memory.NewStore()
	if backend == "sqlite" {
		var err error
		data, err = store.NewSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { data.Close() })
	if err := data.CreateSession(&store.Session{ID: "ses_history"}); err != nil {
		t.Fatal(err)
	}
	return data
}

func seedHistory(t testing.TB, data store.Store, count int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count+3; i++ {
		kind := "text"
		payload := []byte(fmt.Sprintf(`{"type":"text","text":%q}`, strings.Repeat("old ", 1024)))
		state := "completed"
		if i == count || i == count+2 {
			kind = compaction.PartType
			payload = []byte(`{"type":"compaction_marker","version":1,"summary":"saved summary","recent":"recent context"}`)
			if i == count+2 {
				state = "failed"
			}
		}
		role := "user"
		if i == count+1 {
			role = "assistant"
			payload = []byte(`{"type":"text","text":"recent answer"}`)
		}
		id := fmt.Sprintf("msg_%04d", i)
		if err := data.SaveMessage(ctx, store.StoredMessage{ID: id, SessionID: "ses_history", Idx: i, Role: role, State: state, Revision: 1, Parts: []store.StoredPart{{ID: "prt_" + id, MessageID: id, Kind: kind, PayloadJSON: payload}}}); err != nil {
			t.Fatal(err)
		}
		if role == "assistant" || i < count {
			if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_" + id, SessionID: "ses_history", AssistantMessageID: id, Step: i + 1, Status: store.ModelCallStatusCompleted, InputTokens: i + 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type boundedHistoryStore struct{ store.Store }

func (s boundedHistoryStore) ListMessages(context.Context, string) ([]store.StoredMessage, error) {
	return nil, fmt.Errorf("execution loaded the full transcript")
}

func (s boundedHistoryStore) ListModelCalls(context.Context, string) ([]store.ModelCall, error) {
	return nil, fmt.Errorf("execution loaded all model calls")
}

func TestActiveHistoryHydrationAndAbsoluteIndexes(t *testing.T) {
	for _, backend := range []string{"sqlite", "memory"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			seedHistory(t, data, 32)
			client := &requestCaptureClient{}
			sess := New(WithID("ses_history"), WithStore(boundedHistoryStore{data}), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
			if err := sess.activatePlugins(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := sess.hydrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(sess.History()) != 3 || sess.History()[0].ID != "msg_0032" || sess.nextMessageIdx != 35 {
				t.Fatalf("active history = %v, next index = %d", sess.History(), sess.nextMessageIdx)
			}
			if sess.History()[1].Usage == nil || sess.History()[1].Usage.InputTokens != 34 {
				t.Fatal("active model call metadata was not restored")
			}
			for _, input := range []string{"first", "second"} {
				if _, err := sess.Run(t.Context(), input); err != nil {
					t.Fatal(err)
				}
			}
			stored, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil || len(stored) != 39 {
				t.Fatalf("stored messages = %d, %v", len(stored), err)
			}
			for i, message := range stored {
				if message.Idx != i {
					t.Fatalf("message %s index = %d, want %d", message.ID, message.Idx, i)
				}
			}
			calls, err := data.QueryModelCalls(t.Context(), "ses_history", store.ModelCallQuery{FromMessageIndex: 32})
			if err != nil || len(calls) != 3 {
				t.Fatalf("active calls = %d, %v", len(calls), err)
			}
			full := New(WithID("ses_history"), WithStore(data))
			if err := full.hydrate(t.Context()); err != nil || len(full.History()) != 39 {
				t.Fatalf("history without boundary plugin = %d, %v", len(full.History()), err)
			}
		})
	}
}

func TestActiveHistoryModelContextMatchesFullTranscript(t *testing.T) {
	data := historyStore(t, "sqlite")
	seedHistory(t, data, 64)
	// Remove the failed marker from this context-equivalence fixture. Failed
	// markers are covered separately by the storage and hydration assertion.
	full := New(WithID("ses_history"), WithStore(data))
	if err := full.hydrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	baselineClient, boundedClient := &requestCaptureClient{}, &requestCaptureClient{}
	baseline := New(WithClient(baselineClient), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
	baseline.SetHistory(full.History()[:66])
	bounded := New(WithClient(boundedClient), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
	bounded.SetHistory(full.History()[64:66])
	for _, sess := range []*Session{baseline, bounded} {
		if _, err := sess.Run(t.Context(), "continue"); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(baselineClient.request.Messages, boundedClient.request.Messages) {
		t.Fatal("bounded history changed the model request")
	}
	if len(baseline.History()) != 4 || len(bounded.History()) != 4 {
		t.Fatal("execution retained compacted messages")
	}
}

func TestSetActiveHistoryPreservesAbsoluteIndexes(t *testing.T) {
	for _, backend := range []string{"sqlite", "memory"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			seedHistory(t, data, 32)
			sess := New(WithID("ses_history"), WithStore(data), WithClient(&requestCaptureClient{}), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
			if err := sess.activatePlugins(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := sess.hydrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			sess.SetHistory(sess.History())
			if _, err := sess.Run(t.Context(), "continue"); err != nil {
				t.Fatal(err)
			}
			stored, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil || len(stored) != 37 {
				t.Fatalf("stored messages = %d, %v", len(stored), err)
			}
			for i, message := range stored {
				if message.Idx != i {
					t.Fatalf("message %s index = %d, want %d", message.ID, message.Idx, i)
				}
			}
		})
	}
}

func TestActiveHistoryAutomaticCompactionOrdersBoundary(t *testing.T) {
	for _, backend := range []string{"sqlite", "memory"} {
		t.Run(backend, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				data := historyStore(t, backend)
				seedHistory(t, data, 32)
				admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{ID: "run_history", SessionID: "ses_history", Message: "continue"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), admitted.Run.SessionID); err != nil {
					t.Fatal(err)
				}
				sess := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithStore(data), WithClient(&recoveryCompactionClient{}), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 1}), WithPlugin(compaction.New(compaction.WithMinMessages(0))))
				if _, err := sess.runWith(t.Context(), "continue", run.SinkFunc(func(event run.Event) {
					if _, ok := event.(run.IterationStartEvent); ok {
						time.Sleep(time.Second)
					}
				})); err != nil {
					t.Fatal(err)
				}
				stored, err := data.ListMessages(t.Context(), "ses_history")
				if err != nil || len(stored) != 38 {
					t.Fatalf("stored messages = %d, %v", len(stored), err)
				}
				if stored[36].Parts[0].Kind != compaction.PartType || stored[37].Role != "assistant" {
					t.Fatalf("boundary and answer order = %#v", stored[36:])
				}
				if _, err := data.RequeueSessionRun(t.Context(), admitted.Run.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), admitted.Run.SessionID); err != nil {
					t.Fatal(err)
				}
				client := &requestCaptureClient{}
				reopened := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithRunRecovery(), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
				result, err := reopened.Run(t.Context(), "continue")
				if err != nil || result.Response != "done" || len(client.request.Messages) != 0 {
					t.Fatalf("saved answer recovery = %#v, request = %#v, %v", result, client.request, err)
				}
				active := reopened.History()
				if len(active) != 2 || active[0].ID != stored[36].ID || active[1].ID != stored[37].ID {
					t.Fatalf("reopened history = %#v", active)
				}
			})
		})
	}
}

func TestActiveHistoryManualCompactionRetainsArchive(t *testing.T) {
	data := historyStore(t, "sqlite")
	seedHistory(t, data, 32)
	client := &recoveryCompactionClient{}
	sess := New(WithID("ses_history"), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
	for i := 0; i < 2; i++ {
		if _, err := sess.Run(t.Context(), "continue"); err != nil {
			t.Fatal(err)
		}
		if err := sess.RunAction(t.Context(), "compaction.compact", nil, nil); err != nil {
			t.Fatal(err)
		}
		if len(sess.History()) != 1 || sess.History()[0].Content[0].Type() != compaction.PartType {
			t.Fatal("manual compaction retained archived execution history")
		}
	}
	stored, err := data.ListMessages(t.Context(), "ses_history")
	if err != nil || len(stored) != 41 || client.summaries != 2 {
		t.Fatalf("archive = %d, summaries = %d, %v", len(stored), client.summaries, err)
	}
	for i, message := range stored {
		if message.Idx != i {
			t.Fatalf("absolute index = %d, want %d", message.Idx, i)
		}
	}
	reopened := New(WithID("ses_history"), WithStore(data), WithPlugin(compaction.New()))
	if err := reopened.activatePlugins(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := reopened.hydrate(t.Context()); err != nil || len(reopened.History()) != 1 || reopened.nextMessageIdx != 41 {
		t.Fatalf("reopened history = %d, next = %d, %v", len(reopened.History()), reopened.nextMessageIdx, err)
	}
}

func TestRunRecoveryAfterCompactionBoundary(t *testing.T) {
	for _, checkpoint := range []string{"before-boundary", "after-boundary"} {
		t.Run(checkpoint, func(t *testing.T) {
			data := historyStore(t, "sqlite")
			ctx := t.Context()
			admitted, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_history", SessionID: "ses_history", Message: "original input"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, admitted.Run.SessionID); err != nil {
				t.Fatal(err)
			}
			seed := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithStore(data))
			if _, err := seed.persistMessage(ctx, models.Message{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original input"}}}, 0); err != nil {
				t.Fatal(err)
			}
			marker := models.Message{Role: models.RoleUser, Content: models.Content{models.OpaquePart{TypeName: compaction.PartType, Raw: []byte(`{"type":"compaction_marker","version":1,"summary":"tool round already completed","recent":""}`)}}}
			assistant := models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{CallID: "call_old", Name: "old_tool", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "saved result"}}}
			assistantIdx, markerIdx := 1, 2
			if checkpoint == "after-boundary" {
				assistantIdx, markerIdx = 2, 1
				assistant.Content = models.Content{models.TextPart{Text: "saved final answer"}}
			}
			saved, err := seed.persistMessage(ctx, assistant, assistantIdx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := seed.persistMessage(ctx, marker, markerIdx); err != nil {
				t.Fatal(err)
			}
			if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_history", SessionID: "ses_history", RunID: admitted.Run.ID, AssistantMessageID: saved.ID, Step: 1, Status: store.ModelCallStatusCompleted}); err != nil {
				t.Fatal(err)
			}
			if _, err := data.RequeueSessionRun(ctx, admitted.Run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, "ses_history"); err != nil {
				t.Fatal(err)
			}
			client := &requestCaptureClient{}
			sess := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithRunRecovery(), WithStore(boundedHistoryStore{data}), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(compaction.New()))
			result, err := sess.Run(ctx, "original input")
			want := "done"
			if checkpoint == "after-boundary" {
				want = "saved final answer"
			}
			if err != nil || result.Response != want {
				t.Fatalf("recovered = %#v, %v", result, err)
			}
			if checkpoint == "before-boundary" {
				body, _ := json.Marshal(client.request.Messages)
				if strings.Contains(string(body), "saved result") || !strings.Contains(string(body), "tool round already completed") {
					t.Fatalf("recovered context = %s", body)
				}
			} else if len(client.request.Messages) != 0 {
				t.Fatal("saved final response triggered another model request")
			}
			messages, err := data.ListMessages(ctx, "ses_history")
			if err != nil || messages[assistantIdx].ID != saved.ID || messages[assistantIdx].Idx != assistantIdx {
				t.Fatalf("recovery changed assistant identity or index: %v", err)
			}
		})
	}
}

func BenchmarkHistoryHydration(b *testing.B) {
	for _, archived := range []int{32, 1024} {
		for _, bounded := range []bool{false, true} {
			b.Run(fmt.Sprintf("archive=%d/bounded=%t", archived, bounded), func(b *testing.B) {
				data := historyStore(b, "sqlite")
				seedHistory(b, data, archived)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sess := New(WithID("ses_history"), WithStore(data))
					if bounded {
						WithPlugin(compaction.New())(sess)
						if err := sess.activatePlugins(context.Background()); err != nil {
							b.Fatal(err)
						}
					}
					if err := sess.hydrate(context.Background()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestActiveHistoryIgnoresIncompleteBoundary(t *testing.T) {
	boundary := func(state models.MessageState) models.Message {
		return models.Message{State: state, Content: models.Content{models.OpaquePart{TypeName: "boundary", Raw: []byte(`{}`)}}}
	}
	messages := []models.Message{{Role: models.RoleUser}, boundary(models.MessageStateCompleted), {Role: models.RoleAssistant}, boundary(models.MessageStateFailed)}
	active := run.ActiveHistory(messages, "boundary")
	if len(active) != 3 || active[0].State != models.MessageStateCompleted {
		t.Fatal("incomplete boundary discarded valid context")
	}
}
