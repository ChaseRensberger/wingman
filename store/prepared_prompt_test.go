package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPreparedPromptRetainsOriginalRetryIdentity(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			var data store.Store
			if backend == "memory" {
				data = memory.NewStore()
			} else {
				var err error
				data, err = store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
				if err != nil {
					t.Fatal(err)
				}
			}
			defer data.Close()
			sess := &store.Session{ID: "session"}
			if err := data.CreateSession(sess); err != nil {
				t.Fatal(err)
			}
			prepared := "redacted"
			candidate := store.SessionRun{SessionID: sess.ID, RequestID: "request", Message: "secret", Agent: store.Agent{ID: "agent"}, PreparedMessage: &prepared}
			candidate.PreparedAgent = &store.Agent{ID: "agent", ModelRef: "provider/model", Instructions: "captured"}
			first, err := data.AdmitSessionRun(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			if first.Run.Message != "redacted" || first.Run.PreparedMessage != nil || first.Run.PreparedAgent != nil || first.Run.Agent.Instructions != "captured" {
				t.Fatalf("run = %+v", first.Run)
			}
			candidate.PreparedMessage = nil
			candidate.PreparedAgent = nil
			retry, err := data.AdmitSessionRun(context.Background(), candidate)
			if err != nil || retry.Created || retry.Run.ID != first.Run.ID || retry.Run.Message != "redacted" {
				t.Fatalf("retry = %+v, %v", retry, err)
			}
			other := "another transformation"
			candidate.PreparedMessage = &other
			candidate.PreparedAgent = &store.Agent{ID: "agent", Instructions: "changed transform"}
			retry, err = data.AdmitSessionRun(context.Background(), candidate)
			if err != nil || retry.Created || retry.Run.Message != "redacted" || retry.Run.Agent.Instructions != "captured" {
				t.Fatalf("concurrent retry = %+v, %v", retry, err)
			}
			candidate.Message = "changed input"
			if _, err := data.AdmitSessionRun(context.Background(), candidate); !errors.Is(err, store.ErrSessionRunAdmissionConflict) {
				t.Fatalf("conflict = %v", err)
			}
		})
	}
}
