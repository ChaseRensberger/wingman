package tool

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestBashStreamsOutputViaProgress(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	var deltas []string
	var mu sync.Mutex
	progress := NewProgress(func(delta string, _ map[string]any) {
		mu.Lock()
		deltas = append(deltas, delta)
		mu.Unlock()
	})

	inv := Invocation{
		Input:    map[string]any{"command": "echo hello && echo world >&2"},
		WorkDir:  workDir,
		Progress: progress,
	}

	res, err := NewBashTool().Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	combined := strings.Join(deltas, "")
	if !strings.Contains(combined, "hello") || !strings.Contains(combined, "world") {
		t.Fatalf("progress deltas missing expected output: %q", combined)
	}
	if res.Text != combined {
		t.Fatalf("result text %q != combined deltas %q", res.Text, combined)
	}
	if res.Metadata["work_dir"] != workDir || res.Metadata["exit_code"] != 0 {
		t.Fatalf("metadata = %#v", res.Metadata)
	}
}

func TestBashPreservesPartialOutputOnTimeout(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	inv := Invocation{
		Input: map[string]any{
			"command": "echo partial && sleep 10",
			"timeout": "100ms",
		},
		WorkDir: workDir,
	}

	res, err := NewBashTool().Execute(context.Background(), inv)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if !strings.Contains(res.Text, "partial") {
		t.Fatalf("partial output lost: %q", res.Text)
	}
}

func TestBashPreservesPartialOutputOnNonzeroExit(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	inv := Invocation{
		Input:   map[string]any{"command": "echo partial && exit 1"},
		WorkDir: workDir,
	}

	res, err := NewBashTool().Execute(context.Background(), inv)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(res.Text, "partial") {
		t.Fatalf("partial output lost: %q", res.Text)
	}
}

func TestProgressWriterConcurrency(t *testing.T) {
	t.Parallel()
	w := newProgressWriter(nil)

	// Concurrent writes should not race or corrupt.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				w.Write([]byte("x"))
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if w.String() != strings.Repeat("x", 1000) {
		t.Fatalf("concurrent writer corrupted output")
	}
}

func TestProgressSnapshotsMetadata(t *testing.T) {
	nested := map[string]string{"phase": "first"}
	items := []map[string]any{{"value": "first"}}
	metadata := map[string]any{"nested": nested, "items": items}
	var reported map[string]any
	NewProgress(func(_ string, value map[string]any) { reported = value }).Report("", metadata)
	nested["phase"] = "second"
	items[0]["value"] = "second"
	metadata["extra"] = true

	if reported["extra"] != nil || reported["nested"].(map[string]any)["phase"] != "first" || reported["items"].([]any)[0].(map[string]any)["value"] != "first" {
		t.Fatalf("reported metadata mutated: %#v", reported)
	}
}
