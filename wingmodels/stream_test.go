package wingmodels

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestEventStreamPushIterFinal(t *testing.T) {
	s := NewEventStream[StreamPart, *Message](4)
	final := &Message{Role: RoleAssistant, Content: Content{TextPart{Text: "done"}}}

	go func() {
		s.Push(TextStartPart{ID: "1"})
		s.Push(TextDeltaPart{ID: "1", Delta: "hi"})
		s.Push(TextEndPart{ID: "1"})
		s.Close(final, nil)
	}()

	count := 0
	for range s.Iter() {
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}

	got, err := s.Final()
	if err != nil {
		t.Fatal(err)
	}
	if got != final {
		t.Errorf("final mismatch: got=%v want=%v", got, final)
	}
}

func TestEventStreamCloseIdempotent(t *testing.T) {
	s := NewEventStream[StreamPart, *Message](1)
	s.Close(nil, errors.New("first"))
	s.Close(nil, errors.New("second")) // must not panic or overwrite

	_, err := s.Final()
	if err == nil || err.Error() != "first" {
		t.Errorf("expected first close to win, got: %v", err)
	}
}

func TestEventStreamFinalBlocksUntilClose(t *testing.T) {
	s := NewEventStream[StreamPart, *Message](1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = s.Final()
	}()

	select {
	case <-time.After(10 * time.Millisecond):
		// Final should still be blocked.
	}
	s.Close(nil, nil)
	wg.Wait() // Final unblocks after Close.
}

func TestAccumulateBuildsSnapshot(t *testing.T) {
	s := NewEventStream[StreamPart, *Message](16)
	go func() {
		s.Push(StreamStartPart{})
		s.Push(TextStartPart{ID: "t1"})
		s.Push(TextDeltaPart{ID: "t1", Delta: "hello "})
		s.Push(TextDeltaPart{ID: "t1", Delta: "world"})
		s.Push(TextEndPart{ID: "t1"})
		s.Push(ToolInputStartPart{ID: "c1", ToolName: "bash"})
		s.Push(ToolInputDeltaPart{ID: "c1", Delta: `{"cmd":"ls"}`})
		s.Push(ToolInputEndPart{ID: "c1"})
		s.Push(ToolCallPart_{ID: "c1", ToolName: "bash", Input: map[string]any{"cmd": "ls"}})
		s.Push(FinishPart{Reason: FinishReasonToolCalls, Usage: Usage{InputTokens: 10, OutputTokens: 20}})
		s.Close(nil, nil)
	}()

	var lastSnap Snapshot
	for snap, _ := range Accumulate(s) {
		lastSnap = snap
	}

	if lastSnap.Reason != FinishReasonToolCalls {
		t.Errorf("reason: got=%v want=%v", lastSnap.Reason, FinishReasonToolCalls)
	}
	if len(lastSnap.Message.Content) != 2 {
		t.Fatalf("content len: got=%d want=2", len(lastSnap.Message.Content))
	}
	tp, ok := lastSnap.Message.Content[0].(TextPart)
	if !ok || tp.Text != "hello world" {
		t.Errorf("text part: got=%#v", lastSnap.Message.Content[0])
	}
	tc, ok := lastSnap.Message.Content[1].(ToolCallPart)
	if !ok || tc.Name != "bash" || tc.Input["cmd"] != "ls" {
		t.Errorf("tool call: got=%#v", lastSnap.Message.Content[1])
	}
}
