package tool

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

type BashTool struct {
	timeout time.Duration
}

func NewBashTool() *BashTool {
	return &BashTool{
		timeout: 2 * time.Minute,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Execute a bash command and return its output. Use this for running scripts, installing packages, or any shell operations."
}

func (t *BashTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
				"timeout": {
					Type:        "string",
					Description: "Timeout duration (e.g., '30s', '5m'). Defaults to 2 minutes.",
				},
			},
			Required: []string{"command"},
		},
	}
}

func (t *BashTool) DirectoryScoped() {}

func (t *BashTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	command, ok := inv.Input["command"].(string)
	if !ok || command == "" {
		return Result{}, fmt.Errorf("command is required")
	}

	if inv.WorkDir == "" {
		return Result{}, fmt.Errorf("workDir is required for bash tool")
	}

	timeout := t.timeout
	if ts, ok := inv.Input["timeout"].(string); ok && ts != "" {
		if d, err := time.ParseDuration(ts); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = inv.WorkDir

	metadata := map[string]any{"work_dir": inv.WorkDir}
	inv.Progress.Report("", map[string]any{"work_dir": inv.WorkDir})
	w := newProgressWriter(inv.Progress)
	w.output.inv = inv
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	result, outputErr := w.output.finish(metadata)
	err = errors.Join(err, outputErr)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.Metadata["exit_code"] = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %v: %w", timeout, errors.Join(ctx.Err(), outputErr))
		}
		return result, fmt.Errorf("command failed: %w", err)
	}

	result.Metadata["exit_code"] = 0
	return result, nil
}

// progressWriter is a concurrency-safe writer that accumulates output and
// forwards chunks through a tool Progress callback.
type progressWriter struct {
	mu       sync.Mutex
	output   outputCollector
	progress *Progress
}

func newProgressWriter(p *Progress) *progressWriter {
	return &progressWriter{progress: p}
}

func (w *progressWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err = w.output.Write(p)
	delta := string(p)
	if w.progress != nil && delta != "" {
		w.progress.Report(delta, nil)
	}
	return n, err
}

func (w *progressWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.preview.String()
}
