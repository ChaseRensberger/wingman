package run

import (
	"context"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
	"testing"
)

func TestToolVisibilityFiltersAfterHooksAndStillEnforcesPermission(t *testing.T) {
	executed := false
	denied := tool.NewFuncTool("remote_write", "", tool.Definition{
		Name: "remote_write", InputSchema: tool.InputSchema{Type: "object"},
		Permission: &tool.PermissionTarget{Action: "edit"},
	}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executed = true
		return tool.Result{}, nil
	})
	client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		if len(req.Tools) != 1 || req.Tools[0].Name != "read" {
			t.Fatalf("advertised tools = %#v", req.Tools)
		}
		return completedStream(models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_denied", Name: "remote_write", Input: map[string]any{}}}}), nil
	}}
	result, err := Run(t.Context(), Config{
		Client: client, Model: models.ModelRef{Provider: "test", ID: "test"}, MaxSteps: 1,
		WorkDir: t.TempDir(), Tools: []tool.Tool{denied, tool.NewReadTool(), tool.NewWriteTool(), tool.NewEditTool(), tool.NewApplyPatchTool()},
		Permissions: permission.Ruleset{{Action: "edit", Resource: "*", Effect: permission.EffectDeny}},
		Hooks: Hooks{TransformToolDefs: func(_ context.Context, info TransformToolDefsInfo) ([]models.ToolDef, error) {
			if len(info.Tools) != 1 {
				t.Fatalf("pre-hook tools = %#v", info.Tools)
			}
			return append(info.Tools, denied.Definition().AsModelToolDef()), nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed || len(result.Turns) != 1 || !result.Turns[0].Results[0].IsError {
		t.Fatalf("permission bypass: executed=%v result=%#v", executed, result)
	}
}
