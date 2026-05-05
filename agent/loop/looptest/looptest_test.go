package looptest

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/wingmodels"
)

func TestRecordingModelAndSink(t *testing.T) {
	ctx := context.Background()

	m := NewRecordingModel(Reply("hi"), Reply("bye"))
	sink := NewRecordingSink()

	cfg := loop.Config{
		Model:    m,
		Sink:     sink,
		Messages: []wingmodels.Message{wingmodels.NewUserText("start")},
	}

	// First run consumes the "hi" reply.
	if _, err := loop.Run(ctx, cfg); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Second run consumes the "bye" reply.
	if _, err := loop.Run(ctx, cfg); err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	reqs := m.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}

	msgs := sink.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if len(sink.Errors()) != 0 {
		t.Fatalf("expected no errors, got %v", sink.Errors())
	}
}
