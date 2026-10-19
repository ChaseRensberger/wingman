// Package wingcode supplies coding guidance and repository orientation.
package wingcode

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
)

// Plugin adds coding context to the current execution environment.
type Plugin struct{}

// New creates a coding plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the plugin's activation ID.
func (*Plugin) Name() string { return "wingcode" }

// Activate installs coding guidance and refreshes repository orientation per call.
func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
	return nil, r.RegisterTransformSystem(func(ctx context.Context, info run.TransformSystemInfo) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		result := info.System + "\n\n" + guidance
		if info.WorkDir == "" {
			return result, nil
		}
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(probe, "git", "-C", info.WorkDir, "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err != nil {
			return result + "\n\nGit repository: not detected.", nil
		}
		return result + fmt.Sprintf("\n\nGit repository: yes\nRepository root: %s", strings.TrimSpace(string(output))), nil
	})
}

const guidance = `## Coding workflow
Inspect relevant code before making changes. Prefer the smallest correct change and preserve existing style.
Preserve user changes. Do not revert or overwrite work you did not make without explicit permission.
Prefer dedicated tools for reading, searching, and editing files. Use shell commands for builds, tests, and Git inspection when available.
Use only the tools available to this agent. Follow the agent's restrictions on edits and execution.
Avoid destructive commands unless explicitly requested.
Verify meaningful changes with relevant checks. Report what changed, what you ran, and anything you could not verify.`
