// Package orientation supplies general execution and capability context.
package orientation

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/tool"
)

// Reference describes an additional directory available to an agent.
type Reference struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// Config provides additional references advertised to the model.
type Config struct {
	References []Reference
}

// Plugin adds general startup context without coding guidance.
type Plugin struct{ config Config }

// New creates an orientation plugin with a private configuration snapshot.
func New(cfg Config) *Plugin {
	cfg.References = append([]Reference(nil), cfg.References...)
	return &Plugin{config: cfg}
}

// Name returns the plugin's activation ID.
func (*Plugin) Name() string { return "orientation" }

// Activate installs general context in each effective system prompt.
func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
	return nil, r.RegisterTransformSystem(func(ctx context.Context, info run.TransformSystemInfo) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		cfg := p.config
		var out strings.Builder
		out.WriteString(info.System)
		fmt.Fprintf(&out, "\n\n## Execution environment\nPlatform: %s\nTemporary directory: %s\n", runtime.GOOS, os.TempDir())
		if info.WorkDir != "" {
			fmt.Fprintf(&out, "Working directory: %s\n", info.WorkDir)
		}
		fmt.Fprintf(&out, "\n## Your model\nProvider ID: %s\nModel ID: %s\n", info.Model.Provider, info.Model.ID)
		if len(cfg.References) > 0 {
			out.WriteString("\n## Available references\nThese directories provide additional context. Access remains subject to tool permissions.\n")
			for _, ref := range cfg.References {
				fmt.Fprintf(&out, "- %s: %s — %s\n", ref.Name, ref.Path, ref.Description)
			}
		}
		var visible []tool.Tool
		for _, t := range info.Tools {
			if !tool.WhollyDenied(t, info.Permissions) {
				visible = append(visible, t)
			}
		}
		for _, entry := range mcp.Instructions(visible) {
			fmt.Fprintf(&out, "\n## MCP server: %s\n%s\n", entry.Server, entry.Text)
		}
		return out.String(), nil
	})
}
