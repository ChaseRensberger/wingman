package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
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

func TestSQLiteInterruptActiveModelCalls(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_interrupt_calls"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_interrupt_calls", SessionID: "ses_interrupt_calls"})
	if err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_started", SessionID: "ses_interrupt_calls", RunID: run.Run.ID, Step: 1, Status: ModelCallStatusStarted}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_terminal", SessionID: "ses_interrupt_calls", RunID: run.Run.ID, Step: 2, Status: ModelCallStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := data.InterruptActiveModelCalls(ctx, run.Run.ID, "shutdown", "stopped"); err != nil {
		t.Fatal(err)
	}
	calls, err := data.ListModelCalls(ctx, "ses_interrupt_calls")
	if err != nil || calls[0].Status != ModelCallStatusAborted || calls[0].ErrorType != "shutdown" || calls[0].CompletedAt.IsZero() || calls[1].Status != ModelCallStatusCompleted {
		t.Fatalf("calls = %#v, %v", calls, err)
	}
}

func TestSQLiteModelCallEventsReplayAndRollback(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_model_call_events"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_model_call_events", SessionID: "ses_model_call_events"})
	if err != nil {
		t.Fatal(err)
	}
	call := ModelCall{ID: "mcl_event", SessionID: "ses_model_call_events", RunID: run.Run.ID, Step: 1, Status: ModelCallStatusStarted, StructuredOutputJSON: []byte(`{"answer":1}`), MetadataJSON: []byte(`{"trace":"metadata"}`), Trace: []byte(`{"trace":"metadata"}`)}
	if err := data.UpsertModelCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := data.InterruptActiveModelCalls(ctx, run.Run.ID, "shutdown", "stopped"); err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: call.SessionID}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectSessionModelCalls(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.ListModelCalls(ctx, call.SessionID)
	if err != nil || !reflect.DeepEqual(projected, stored) {
		t.Fatalf("projected = %#v, stored = %#v, error = %v", projected, stored, err)
	}
	if len(events) != 4 || events[2].Type != EventSessionModelCallSaved || events[3].Type != EventSessionModelCallSaved {
		t.Fatalf("events = %#v", events)
	}

	err = data.UpsertModelCall(ctx, ModelCall{ID: "mcl_rollback", SessionID: call.SessionID, Step: 2, StructuredOutputJSON: []byte(`{`)})
	if err == nil {
		t.Fatal("UpsertModelCall succeeded with invalid opaque JSON")
	}
	events, err = data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: call.SessionID}, 0, 10)
	if err != nil || len(events) != 4 {
		t.Fatalf("events after rollback = %#v, error = %v", events, err)
	}
	stored, err = data.ListModelCalls(ctx, call.SessionID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("calls after rollback = %#v, error = %v", stored, err)
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

func TestSQLiteToolUseLifecycleAndInterruption(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_tools"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_tools", SessionID: "ses_tools"})
	if err != nil {
		t.Fatal(err)
	}
	use := ToolUse{ID: "tlu_test", SessionID: "ses_tools", RunID: run.Run.ID, Step: 1, Ordinal: 0, CallID: "call_1", Name: "read", Status: ToolUseStatusProposed, InputJSON: []byte(`{"path":"a"}`)}
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveToolUse(ctx, ToolUse{ID: use.ID, SessionID: use.SessionID, RunID: use.RunID, Step: 1, Ordinal: 0, CallID: use.CallID, Name: use.Name, Status: ToolUseStatusStarted}); !errors.Is(err, ErrToolUseInvalidTransition) {
		t.Fatalf("skip error = %v", err)
	}
	use.Status = ToolUseStatusAuthorized
	use.InputJSON = []byte(`{"path":"rewritten"}`)
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status = ToolUseStatusStarted
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	if err := data.InterruptActiveToolUses(ctx); err != nil {
		t.Fatal(err)
	}
	uses, err := data.ListToolUses(ctx, "ses_tools")
	if err != nil || len(uses) != 1 || uses[0].Status != ToolUseStatusInterrupted || uses[0].ErrorType != "process_interrupted" || uses[0].CompletedAt.IsZero() {
		t.Fatalf("uses = %#v, error = %v", uses, err)
	}
	events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: "ses_tools"}, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectSessionToolUses(events)
	if err != nil || !reflect.DeepEqual(projected, uses) {
		t.Fatalf("projected uses = %#v, stored = %#v, error = %v", projected, uses, err)
	}
	interrupted := uses[0]
	interrupted.Status = ToolUseStatusCompleted
	if err := data.SaveToolUse(ctx, interrupted); !errors.Is(err, ErrToolUseInvalidTransition) {
		t.Fatalf("terminal transition error = %v", err)
	}
	if err := data.SaveToolUse(ctx, ToolUse{ID: "tlu_conflict", SessionID: "ses_tools", RunID: run.Run.ID, Step: 1, Ordinal: 0, CallID: "call_2", Name: "write", Status: ToolUseStatusProposed}); !errors.Is(err, ErrToolUseIdentityConflict) {
		t.Fatalf("identity conflict error = %v", err)
	}
}

func TestSQLiteToolUsePersistsStructuredResultSeparately(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_tool_result"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_tool_result", SessionID: "ses_tool_result"})
	if err != nil {
		t.Fatal(err)
	}
	use := ToolUse{ID: "tlu_result", SessionID: "ses_tool_result", RunID: run.Run.ID, Step: 1, Ordinal: 0, CallID: "call_1", Name: "search", Status: ToolUseStatusProposed}
	for _, status := range []string{ToolUseStatusProposed, ToolUseStatusAuthorized, ToolUseStatusStarted, ToolUseStatusCompleted} {
		use.Status = status
		if status == ToolUseStatusCompleted {
			use.Output = "one result"
			use.StructuredJSON = []byte(`{"count":1}`)
			use.MetadataJSON = []byte(`{"source":"test"}`)
		}
		if err := data.SaveToolUse(ctx, use); err != nil {
			t.Fatal(err)
		}
	}
	uses, err := data.ListToolUses(ctx, use.SessionID)
	if err != nil || len(uses) != 1 || string(uses[0].StructuredJSON) != `{"count":1}` || string(uses[0].MetadataJSON) != `{"source":"test"}` {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
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

func TestSQLiteSaveMessageRevisionedAndRollback(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_messages"}); err != nil {
		t.Fatal(err)
	}
	initial := StoredMessage{ID: "msg_one", SessionID: "ses_messages", Idx: 1, Role: "assistant", Parts: []StoredPart{{ID: "part_one", MessageID: "msg_one", Sequence: 1, Kind: "text", PayloadJSON: []byte(`{"text":"one"}`)}}}
	if err := data.SaveMessage(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, initial); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	conflict := initial
	conflict.Parts[0].PayloadJSON = []byte(`{"text":"changed"}`)
	if err := data.SaveMessage(ctx, conflict); !errors.Is(err, ErrMessageRevisionConflict) {
		t.Fatalf("conflict = %v", err)
	}
	updated := initial
	updated.Revision = 2
	updated.Parts = []StoredPart{{ID: "part_two", MessageID: "msg_one", Sequence: 1, Kind: "text", PayloadJSON: []byte(`{"text":"two"}`)}}
	if err := data.SaveMessage(ctx, updated); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, initial); !errors.Is(err, ErrMessageRevisionStale) {
		t.Fatalf("stale = %v", err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER reject_part BEFORE INSERT ON parts WHEN NEW.id = 'part_bad' BEGIN SELECT RAISE(ABORT, 'reject'); END`); err != nil {
		t.Fatal(err)
	}
	failed := updated
	failed.Revision = 3
	failed.State = "streaming"
	failed.Parts = []StoredPart{{ID: "part_bad", MessageID: "msg_one", Sequence: 1, Kind: "text", PayloadJSON: []byte(`{}`)}}
	if err := data.SaveMessage(ctx, failed); err == nil {
		t.Fatal("trigger save succeeded")
	}
	messages, err := data.ListMessages(ctx, "ses_messages")
	if err != nil || len(messages) != 1 || messages[0].Revision != 2 || messages[0].State != "completed" || len(messages[0].Parts) != 1 || messages[0].Parts[0].ID != "part_two" {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
}

func TestSQLiteSaveMessageRejectsInvalidOwnershipAndIndex(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	ctx := context.Background()
	message := StoredMessage{ID: "msg_one", SessionID: "ses_messages", Idx: 1, Role: "user", Parts: []StoredPart{{ID: "part_one", MessageID: "msg_one", Kind: "text", PayloadJSON: []byte(`{}`)}}}
	if err := data.SaveMessage(ctx, message); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing parent = %v", err)
	}
	if err := data.CreateSession(&Session{ID: "ses_messages"}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, StoredMessage{ID: "msg_two", SessionID: "ses_messages", Idx: 1, Role: "user"}); err == nil {
		t.Fatal("duplicate index succeeded")
	}
	if err := data.SaveMessage(ctx, StoredMessage{ID: "msg_two", SessionID: "ses_messages", Idx: 2, Role: "user", Parts: []StoredPart{{ID: "part_one", MessageID: "msg_two", Kind: "text", PayloadJSON: []byte(`{}`)}}}); err == nil {
		t.Fatal("part ownership conflict succeeded")
	}
}

func TestSQLiteMessagesMayBelongToRunOrSession(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_message_run"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_message", SessionID: "ses_message_run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []StoredMessage{{ID: "msg_direct", SessionID: "ses_message_run", Idx: 1, Role: "user"}, {ID: "msg_run", SessionID: "ses_message_run", RunID: run.Run.ID, Idx: 2, Role: "assistant"}} {
		if err := data.SaveMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	changed := StoredMessage{ID: "msg_run", SessionID: "ses_message_run", Idx: 2, Role: "assistant", Revision: 2}
	if err := data.SaveMessage(ctx, changed); err == nil {
		t.Fatal("run identity rewrite succeeded")
	}
	if _, err := data.db.Exec(`DELETE FROM session_runs WHERE id = ?`, run.Run.ID); err != nil {
		t.Fatal(err)
	}
	messages, err := data.ListMessages(ctx, "ses_message_run")
	if err != nil || len(messages) != 1 || messages[0].ID != "msg_direct" || messages[0].RunID != "" {
		t.Fatalf("messages = %#v, %v", messages, err)
	}
}

func TestSQLiteSaveMessageRevisionRaceAcrossHandles(t *testing.T) {
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
	if err := first.CreateSession(&Session{ID: "ses_race"}); err != nil {
		t.Fatal(err)
	}
	base := StoredMessage{ID: "msg_race", SessionID: "ses_race", Idx: 1, Role: "assistant", Parts: []StoredPart{{ID: "part_base", MessageID: "msg_race", Kind: "text", PayloadJSON: []byte(`{}`)}}}
	if err := first.SaveMessage(ctx, base); err != nil {
		t.Fatal(err)
	}
	v2 := base
	v2.Revision = 2
	v2.Parts = []StoredPart{{ID: "part_two", MessageID: "msg_race", Kind: "text", PayloadJSON: []byte(`{"revision":2}`)}}
	v3 := base
	v3.Revision = 3
	v3.Parts = []StoredPart{{ID: "part_three", MessageID: "msg_race", Kind: "text", PayloadJSON: []byte(`{"revision":3}`)}}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, save := range []struct {
		data    *SQLiteStore
		message StoredMessage
	}{{first, v2}, {second, v3}} {
		wg.Add(1)
		go func(save struct {
			data    *SQLiteStore
			message StoredMessage
		}) {
			defer wg.Done()
			<-start
			errs <- save.data.SaveMessage(ctx, save.message)
		}(save)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrMessageRevisionStale) {
			t.Fatalf("race error = %v", err)
		}
	}
	messages, err := first.ListMessages(ctx, "ses_race")
	if err != nil || len(messages) != 1 || messages[0].Revision != 3 || len(messages[0].Parts) != 1 || messages[0].Parts[0].ID != "part_three" {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
}
