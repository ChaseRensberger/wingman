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
