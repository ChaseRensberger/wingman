package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteModelCallAllowsUnavailableCost(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(context.Background(), ModelCall{
		SessionID: "ses_test",
		Step:      1,
		Status:    ModelCallStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Cost != nil {
		t.Fatalf("calls = %#v, want one call with unavailable cost", calls)
	}
}
