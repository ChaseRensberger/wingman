// Projectcontext adds project notes to model requests and exposes a read tool.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

func main() {
	err := plugin.Serve(context.Background(), plugin.Definition{
		ID: "example/project-context", Name: "Project context", Version: "v1.0.0",
		Setup: func(ctx context.Context, host *plugin.Host, config map[string]any) (*plugin.Extension, error) {
			return &plugin.Extension{
				Tools: []plugin.Tool{{
					Tool: protocol.Tool{Name: "project_notes", Description: "Read PROJECT.md in the session directory", InputSchema: map[string]any{"type": "object", "additionalProperties": false}, DirectoryScoped: true, Permission: &tool.PermissionTarget{Action: "read"}},
					Execute: func(ctx context.Context, input tool.Invocation) (tool.Result, error) {
						data, err := os.ReadFile(filepath.Join(input.WorkDir, "PROJECT.md"))
						return tool.Result{Text: string(data)}, err
					},
				}},
				Hooks: run.Hooks{TransformSystem: func(ctx context.Context, input run.TransformSystemInfo) (string, error) {
					if instruction, ok := config["instruction"].(string); ok && instruction != "" {
						return input.System + "\n\n" + instruction, nil
					}
					return input.System, nil
				}},
			}, nil
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
