package session

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
)

func TestLoopLogsRetainExecutionCorrelation(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})).With(
		"session_id", "ses_log",
		"run_id", "run_log",
		"agent_id", "agt_log",
		"scope_id", "scope_log",
	)

	logLoopEvent(logger, run.IterationStartEvent{Step: 1})
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"session_id": "ses_log", "run_id": "run_log", "agent_id": "agt_log", "scope_id": "scope_log"} {
		if entry[key] != want {
			t.Errorf("%s = %v, want %q", key, entry[key], want)
		}
	}
}
