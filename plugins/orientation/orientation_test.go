package orientation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPInstructionsRequireSelectedPermittedTools(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "manuals", Version: "1"}, &mcpsdk.ServerOptions{Instructions: "Use the manual index before searching."})
	server.AddTool(&mcpsdk.Tool{Name: "lookup", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{}, nil
	})
	host := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil))
	defer host.Close()
	manager := wingmcp.New(t.Context(), wingmcp.Config{Servers: map[string]wingmcp.ServerConfig{"manuals": {Type: "remote", URL: host.URL}}})
	defer manager.Close()
	tools := manager.Tools()
	if len(tools) != 1 {
		t.Fatalf("MCP tools = %v, status=%v", tools, manager.Status())
	}
	for _, tc := range []struct {
		name                   string
		selected, denied, want bool
	}{
		{"selected", true, false, true}, {"unselected", false, false, false}, {"denied", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := run.TransformSystemInfo{System: "agent rules", Model: models.ModelRef{Provider: "test", ID: "first"}}
			if tc.selected {
				info.Tools = tools
			}
			if tc.denied {
				info.Permissions = permission.Ruleset{{Action: "manuals_*", Resource: "*", Effect: permission.EffectDeny}}
			}
			generation, err := plugin.ActivateAll(New(Config{}))
			if err != nil {
				t.Fatal(err)
			}
			defer generation.Close(t.Context())
			text, err := generation.Runtime().Hooks.TransformSystem(t.Context(), info)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(text, "Use the manual index") != tc.want {
				t.Fatalf("instructions=%q want=%v", text, tc.want)
			}
			if strings.Contains(text, "Git repository") || strings.Contains(text, "Coding workflow") {
				t.Fatalf("coding context in general plugin: %s", text)
			}
			info.Model.ID = "second"
			text, err = generation.Runtime().Hooks.TransformSystem(t.Context(), info)
			if err != nil || !strings.Contains(text, "Model ID: second") || strings.Contains(text, "Model ID: first") {
				t.Fatalf("stale model identity: %s err=%v", text, err)
			}
		})
	}
}
