package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

func TestSQLiteModelCallPreservesTimestampPrecision(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, time.July, 27, 4, 0, 0, 123456789, time.UTC)
	completedAt := startedAt.Add(987 * time.Millisecond)
	if err := data.UpsertModelCall(context.Background(), ModelCall{
		SessionID:   "ses_test",
		Step:        1,
		Status:      ModelCallStatusCompleted,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if !calls[0].StartedAt.Equal(startedAt) || !calls[0].CompletedAt.Equal(completedAt) {
		t.Fatalf("timestamps = %s to %s, want %s to %s", calls[0].StartedAt, calls[0].CompletedAt, startedAt, completedAt)
	}
}

func TestSQLiteSeedsWingAgent(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	agents, err := data.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("seeded agents = %d, want 1", len(agents))
	}

	for _, agent := range agents {
		if agent.Name != "WingAgent" {
			continue
		}
		if agent.Instructions != wingAgentInstructions {
			t.Fatal("WingAgent instructions do not match the default")
		}
		if got, want := agent.Tools, []string{"read", "grep", "glob", "write", "edit", "bash", "webfetch", "websearch", "question"}; !slices.Equal(got, want) {
			t.Fatalf("Friday tools = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("WingAgent was not seeded")
}
