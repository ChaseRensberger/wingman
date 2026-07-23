package store

import (
	"context"
	"path/filepath"
	"slices"
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

func TestSQLiteSeedsFridayAgent(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	agents, err := data.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 3 {
		t.Fatalf("seeded agents = %d, want 3", len(agents))
	}

	for _, agent := range agents {
		if agent.Name != "Friday" {
			continue
		}
		if agent.Instructions != fridayAgentInstructions {
			t.Fatal("Friday instructions do not match the default")
		}
		if got, want := agent.Tools, []string{"webfetch", "websearch"}; !slices.Equal(got, want) {
			t.Fatalf("Friday tools = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("Friday agent was not seeded")
}
