package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestSQLiteModelCallsAreScopedToRuns(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	first, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_first", SessionID: "ses_test"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_second", SessionID: "ses_test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []ModelCall{
		{ID: "mcl_first", SessionID: "ses_test", RunID: first.Run.ID, Step: 1, Status: ModelCallStatusStarted},
		{ID: "mcl_second", SessionID: "ses_test", RunID: second.Run.ID, Step: 1, Status: ModelCallStatusStarted},
	} {
		if err := data.UpsertModelCall(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := data.ListModelCalls(ctx, "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].RunID == calls[1].RunID {
		t.Fatalf("calls = %#v, want two step-one calls from distinct runs", calls)
	}
}

func TestSQLiteModelCallUpsertUsesStableID(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_test", SessionID: "ses_test"})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	createdAt := startedAt.Add(time.Second)
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_test", SessionID: "ses_test", RunID: run.Run.ID, Step: 1, Status: ModelCallStatusStarted, StartedAt: startedAt, CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_test", SessionID: "ses_other", RunID: "run_other", Step: 9, Attempt: 2, Status: ModelCallStatusCompleted, ProviderRequestID: "request_123", CompletedAt: startedAt.Add(time.Minute), StartedAt: startedAt.Add(time.Hour), CreatedAt: createdAt.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_conflict", SessionID: "ses_test", RunID: run.Run.ID, Step: 1, Status: ModelCallStatusFailed}); !errors.Is(err, ErrModelCallAttemptConflict) {
		t.Fatalf("conflict error = %v, want %v", err, ErrModelCallAttemptConflict)
	}
	calls, err := data.ListModelCalls(ctx, "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %#v, want one", calls)
	}
	call := calls[0]
	if call.RunID != run.Run.ID || call.Step != 1 || call.Attempt != 1 || !call.StartedAt.Equal(startedAt) || !call.CreatedAt.Equal(createdAt) {
		t.Fatalf("immutable identity = %#v", call)
	}
	if call.Status != ModelCallStatusCompleted || call.ProviderRequestID != "request_123" || call.CompletedAt.IsZero() {
		t.Fatalf("terminal fields = %#v", call)
	}
}

func TestSQLiteModelCallConcurrentAttemptConflictAcrossHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	ctx := context.Background()
	if err := first.CreateSession(&Session{ID: "ses_concurrent_call"}); err != nil {
		t.Fatal(err)
	}
	admission, err := first.AdmitSessionRun(ctx, SessionRun{ID: "run_concurrent_call", SessionID: "ses_concurrent_call"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, data := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(i int, data *SQLiteStore) {
			defer wg.Done()
			<-start
			errs <- data.UpsertModelCall(ctx, ModelCall{ID: fmt.Sprintf("mcl_concurrent_%d", i), SessionID: "ses_concurrent_call", RunID: admission.Run.ID, Step: 1, Status: ModelCallStatusStarted})
		}(i, data)
	}
	close(start)
	wg.Wait()
	close(errs)
	var succeeded, conflicted int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrModelCallAttemptConflict):
			conflicted++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestSQLiteModelCallChronology(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	for _, call := range []ModelCall{
		{ID: "mcl_b", SessionID: "ses_test", Step: 9, Status: ModelCallStatusCompleted, ContextTokens: 1, StartedAt: base},
		{ID: "mcl_a", SessionID: "ses_test", Step: 1, Status: ModelCallStatusCompleted, ContextTokens: 2, StartedAt: base},
		{ID: "mcl_c", SessionID: "ses_test", Step: 1, Status: ModelCallStatusCompleted, ContextTokens: 3, StartedAt: base.Add(time.Minute)},
	} {
		if err := data.UpsertModelCall(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := data.ListModelCalls(ctx, "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{calls[0].ID, calls[1].ID, calls[2].ID}; !slices.Equal(got, []string{"mcl_a", "mcl_b", "mcl_c"}) {
		t.Fatalf("order = %v", got)
	}
	latest, err := data.LatestModelCall(ctx, "ses_test")
	if err != nil || latest == nil || latest.ID != "mcl_c" {
		t.Fatalf("latest = %#v, error = %v", latest, err)
	}
}

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
