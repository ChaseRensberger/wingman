package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSessionRunsClaimInSequence(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"first", "second"} {
		if _, err := data.CreateSessionRun(context.Background(), SessionRun{
			SessionID: "ses_test",
			Message:   message,
			Agent:     Agent{ID: "agt_test", ModelRef: "openai/gpt-5.6-terra"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"first", "second"} {
		run, err := data.ClaimNextSessionRun(context.Background(), "ses_test")
		if err != nil {
			t.Fatal(err)
		}
		if run == nil || run.Message != want || run.Status != SessionRunStatusRunning {
			t.Fatalf("claimed run = %#v, want running %q", run, want)
		}
		if err := data.CompleteSessionRun(context.Background(), run.ID, SessionRunStatusCompleted, ""); err != nil {
			t.Fatal(err)
		}
	}
	run, err := data.ClaimNextSessionRun(context.Background(), "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Fatalf("claimed unexpected run: %#v", run)
	}
}
