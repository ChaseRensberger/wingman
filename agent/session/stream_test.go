package session

import (
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
)

func TestClassifyToolProgress(t *testing.T) {
	event := run.ToolExecutionProgressEvent{
		CallID:      "call_test",
		Name:        "bash",
		OutputDelta: "partial",
		Metadata:    map[string]any{"phase": "running"},
	}

	typ, data := classify(event)
	if typ != "tool_progress" {
		t.Fatalf("type = %q, want tool_progress", typ)
	}
	progress, ok := data.(run.ToolExecutionProgressEvent)
	if !ok || progress.CallID != event.CallID || progress.OutputDelta != event.OutputDelta {
		t.Fatalf("data = %#v, want %#v", data, event)
	}
}

func TestClassifyToolUseLifecycle(t *testing.T) {
	call := run.ToolCall{ID: "call_test", ToolUseID: "tlu_test", Name: "bash", Args: map[string]any{"command": "pwd"}}
	for _, test := range []struct {
		event run.Event
		want  string
	}{
		{run.ToolUseProposedEvent{Call: call}, "tool_proposed"},
		{run.ToolUseAuthorizedEvent{Call: call}, "tool_authorized"},
	} {
		typ, data := classify(test.event)
		if typ != test.want {
			t.Errorf("type = %q, want %q", typ, test.want)
		}
		switch data.(type) {
		case run.ToolUseProposedEvent, run.ToolUseAuthorizedEvent:
		default:
			t.Errorf("data = %#v, want tool use lifecycle event", data)
		}
	}
}
