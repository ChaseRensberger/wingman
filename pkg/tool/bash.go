package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"wingman/pkg/models"
)

type BashTool struct {
	workDir string
	timeout time.Duration
}

func NewBashTool(workDir string) *BashTool {
	return &BashTool{
		workDir: workDir,
		timeout: 2 * time.Minute,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Execute a bash command and return its output. Use this for running scripts, installing packages, or any shell operations."
}

func (t *BashTool) Definition() models.WingmanToolDefinition {
	return models.WingmanToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: models.WingmanToolInputSchema{
			Type: "object",
			Properties: map[string]models.WingmanToolProperty{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
				"workdir": {
					Type:        "string",
					Description: "Working directory for the command (optional, defaults to current directory)",
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

func (t *BashTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required")
	}

	workDir := t.workDir
	if wd, ok := params["workdir"].(string); ok && wd != "" {
		workDir = wd
	}

	timeout := t.timeout
	if ts, ok := params["timeout"].(string); ok && ts != "" {
		if d, err := time.ParseDuration(ts); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("command timed out after %v", timeout)
		}
		return output, fmt.Errorf("command failed: %w", err)
	}

	return output, nil
}
