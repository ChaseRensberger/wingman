package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestRunRecoveryDistinguishesShutdownFromCancellation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		name := "shutdown"
		if cancelRun {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			dispatched := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if requests.Add(1) == 1 {
					close(dispatched)
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			data := memory.NewStore()
			ctx := context.Background()
			if err := data.CreateSession(&store.Session{ID: "ses_restart"}); err != nil {
				t.Fatal(err)
			}
			agent := store.Agent{ID: "agt_restart", ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL}}}
			admitted, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_restart", Agent: agent, Message: "hello"})
			if err != nil {
				t.Fatal(err)
			}
			first := New(Config{Store: data})
			defer first.Close(ctx)
			if err := first.Start(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-dispatched:
			case <-time.After(5 * time.Second):
				t.Fatal("provider did not start")
			}
			if cancelRun {
				if n, err := first.runs.abort(ctx, "ses_restart", admitted.Run.ID); n != 1 || err != nil {
					t.Fatalf("cancel = %d, %v", n, err)
				}
			}
			if err := first.Close(ctx); err != nil {
				t.Fatal(err)
			}
			before, err := data.GetSessionRun(ctx, "ses_restart", admitted.Run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := store.SessionRunStatusRunning
			if cancelRun {
				want = store.SessionRunStatusAborted
			}
			if before.Status != want {
				t.Fatalf("before restart = %#v", before)
			}
			second := New(Config{Store: data})
			defer second.Close(ctx)
			if err := second.Start(ctx); err != nil {
				t.Fatal(err)
			}
			deadline := time.After(5 * time.Second)
			for {
				current, err := data.GetSessionRun(ctx, "ses_restart", admitted.Run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if cancelRun {
					if current.Status != store.SessionRunStatusAborted || current.RecoveryAttempts != 0 || requests.Load() != 1 {
						t.Fatalf("cancellation revived: %#v", current)
					}
					break
				}
				if current.Status == store.SessionRunStatusCompleted {
					if current.RecoveryAttempts != 1 || requests.Load() != 2 {
						t.Fatalf("recovered run = %#v, requests = %d", current, requests.Load())
					}
					break
				}
				if current.Status == store.SessionRunStatusFailed {
					t.Fatalf("recovery failed: %#v", current)
				}
				select {
				case <-deadline:
					t.Fatal("recovery did not finish")
				case <-time.After(time.Millisecond):
				}
			}
			messages, err := data.ListMessages(ctx, "ses_restart")
			if err != nil {
				t.Fatal(err)
			}
			users := 0
			for _, message := range messages {
				if message.Role == "user" {
					users++
				}
			}
			if users != 1 {
				t.Fatalf("input duplicated: %d messages", users)
			}
		})
	}
}

func TestRunRecoveryBudgetSurvivesReopeningStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wingman.db")
	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_budget"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_budget", SessionID: "ses_budget"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(ctx, "ses_budget"); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= store.MaxRunRecoveryAttempts+1; attempt++ {
		data, err := store.NewSQLiteStore(path)
		if err != nil {
			t.Fatal(err)
		}
		server := New(Config{Store: &startupRecoveryStore{Store: data}})
		if err := server.Start(ctx); err != nil {
			t.Fatal(err)
		}
		current, err := data.GetSessionRun(ctx, "ses_budget", "run_budget")
		if err != nil {
			t.Fatal(err)
		}
		if attempt > store.MaxRunRecoveryAttempts {
			if current.Status != store.SessionRunStatusAborted || current.ErrorType != "recovery_exhausted" {
				t.Fatalf("budget did not stop recovery: %#v", current)
			}
		} else {
			if current.Status != store.SessionRunStatusQueued || current.RecoveryAttempts != attempt {
				t.Fatalf("attempt %d: %#v", attempt, current)
			}
			if _, err := data.ClaimNextSessionRun(ctx, "ses_budget"); err != nil {
				t.Fatal(err)
			}
		}
		if err := server.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err := data.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunRecoveryCleansCancelledRecordsWithoutResuming(t *testing.T) {
	for _, backend := range []string{"sqlite", "memory"} {
		for _, boundary := range []string{"model", "message", "tool"} {
			t.Run(backend+"/"+boundary, func(t *testing.T) {
				ctx := context.Background()
				path := filepath.Join(t.TempDir(), "wingman.db")
				var data store.Store = memory.NewStore()
				var err error
				if backend == "sqlite" {
					data, err = store.NewSQLiteStore(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := data.CreateSession(&store.Session{ID: "ses_cancel_crash"}); err != nil {
					t.Fatal(err)
				}
				admitted, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_cancel_crash", SessionID: "ses_cancel_crash", Message: "hello"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(ctx, admitted.Run.SessionID); err != nil {
					t.Fatal(err)
				}
				message := store.StoredMessage{ID: "msg_cancel_crash", SessionID: admitted.Run.SessionID, RunID: admitted.Run.ID, Role: "assistant", State: "in_progress", Revision: 1, Parts: []store.StoredPart{{ID: "prt_cancel_crash", MessageID: "msg_cancel_crash", Kind: "text", PayloadJSON: []byte(`{"type":"text","text":"partial"}`)}}}
				call := store.ModelCall{ID: "mcl_cancel_crash", SessionID: admitted.Run.SessionID, RunID: admitted.Run.ID, AssistantMessageID: message.ID, Step: 1, Status: store.ModelCallStatusCompleted}
				if boundary == "model" {
					message.State, call.Status = "failed", store.ModelCallStatusStarted
				}
				if boundary == "tool" {
					message.Parts[0].Kind = "tool"
					message.Parts[0].PayloadJSON = []byte(`{"type":"tool","call_id":"call_tool","name":"write","state":"pending","input":{}}`)
				}
				if err := data.SaveMessage(ctx, message); err != nil {
					t.Fatal(err)
				}
				if err := data.UpsertModelCall(ctx, call); err != nil {
					t.Fatal(err)
				}
				if boundary == "tool" {
					use := store.ToolUse{ID: "tlu_cancel_crash", SessionID: admitted.Run.SessionID, RunID: admitted.Run.ID, ModelCallID: call.ID, AssistantMessageID: message.ID, PartID: message.Parts[0].ID, Step: 1, Ordinal: 1, Name: "write", CallID: "call_tool", InputJSON: []byte(`{}`)}
					for _, status := range []string{store.ToolUseStatusProposed, store.ToolUseStatusAuthorized, store.ToolUseStatusStarted} {
						use.Status = status
						if err := data.SaveToolUse(ctx, use); err != nil {
							t.Fatal(err)
						}
					}
				}
				first := New(Config{Store: data})
				first.runs.runIDs[admitted.Run.SessionID] = admitted.Run.ID
				first.runs.runCancel[admitted.Run.SessionID] = func() {}
				if n, err := first.runs.abort(ctx, admitted.Run.SessionID, admitted.Run.ID); n != 1 || err != nil {
					t.Fatalf("cancel = %d, %v", n, err)
				}
				cancelled, err := data.GetSessionRun(ctx, admitted.Run.SessionID, admitted.Run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := first.Close(ctx); err != nil {
					t.Fatal(err)
				}
				// The worker never settles its records, as when the process exits after cancellation commits.
				if backend == "sqlite" {
					if err := data.Close(); err != nil {
						t.Fatal(err)
					}
					data, err = store.NewSQLiteStore(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				defer data.Close()
				second := New(Config{Store: data})
				defer second.Close(ctx)
				if err := second.Start(ctx); err != nil {
					t.Fatal(err)
				}
				current, err := data.GetSessionRun(ctx, admitted.Run.SessionID, admitted.Run.ID)
				if err != nil || current.Status != store.SessionRunStatusAborted || current.ErrorType != "cancelled" || current.RecoveryAttempts != 0 || !current.CompletedAt.Equal(cancelled.CompletedAt) {
					t.Fatalf("cancelled run changed: %#v, %v", current, err)
				}
				calls, err := data.ListModelCalls(ctx, admitted.Run.SessionID)
				if err != nil || len(calls) != 1 || calls[0].Status == store.ModelCallStatusStarted {
					t.Fatalf("model records = %#v, %v", calls, err)
				}
				messages, err := data.ListMessages(ctx, admitted.Run.SessionID)
				if err != nil || len(messages) != 1 || messages[0].State != "failed" {
					t.Fatalf("messages = %#v, %v", messages, err)
				}
				if boundary == "tool" {
					uses, err := data.ListToolUses(ctx, admitted.Run.SessionID)
					if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted {
						t.Fatalf("tool records = %#v, %v", uses, err)
					}
				}
				pending, err := data.ListSessionRunsForRecovery(ctx)
				if err != nil || len(pending) != 0 {
					t.Fatalf("unfinished runs = %#v, %v", pending, err)
				}
				before, err := data.GetSession(admitted.Run.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err := second.recoverStartup(ctx); err != nil {
					t.Fatal(err)
				}
				after, err := data.GetSession(admitted.Run.SessionID)
				if err != nil || after.AggregateVersion != before.AggregateVersion {
					t.Fatalf("second recovery changed settled records: %#v, %v", after, err)
				}
			})
		}
	}
}
