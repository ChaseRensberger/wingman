package run

import (
	"testing"

	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

func TestCheckPermissionDenyReturnsStructuredMetadata(t *testing.T) {
	r := &runner{cfg: Config{Permissions: permission.Ruleset{{Action: "bash", Resource: "*", Effect: permission.EffectDeny}}}}
	res, ok := r.checkPermission(ToolCall{ID: "call_1", Name: "bash", Args: map[string]any{"command": "pwd"}})
	if ok {
		t.Fatal("permission check allowed denied command")
	}
	if !res.IsError || res.Output != "permission denied: bash pwd" {
		t.Fatalf("result = %#v", res)
	}
	meta := permissionMetadata(t, res.Metadata)
	if meta["effect"] != "deny" || meta["action"] != "bash" || meta["resource"] != "pwd" {
		t.Fatalf("permission metadata = %#v", meta)
	}
}

func TestCheckPermissionAskReturnsStructuredMetadata(t *testing.T) {
	r := &runner{cfg: Config{Permissions: permission.Ruleset{{Action: "bash", Resource: "*", Effect: permission.EffectAsk}}}}
	res, ok := r.checkPermission(ToolCall{ID: "call_1", Name: "bash", Args: map[string]any{"command": "pwd"}})
	if ok {
		t.Fatal("permission check allowed ask command")
	}
	if !res.IsError || res.Output != "permission required: bash pwd" {
		t.Fatalf("result = %#v", res)
	}
	meta := permissionMetadata(t, res.Metadata)
	if meta["effect"] != "ask" || meta["action"] != "bash" || meta["resource"] != "pwd" {
		t.Fatalf("permission metadata = %#v", meta)
	}
}

func TestPermissionTargetMapsMutatingToolsToEdit(t *testing.T) {
	action, resources, err := permissionTarget(ToolCall{Name: "write", Args: map[string]any{"filePath": "docs/index.md"}}, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if action != "edit" || len(resources) != 1 || resources[0] != "docs/index.md" {
		t.Fatalf("target = %s %#v", action, resources)
	}
}

func TestPermissionTargetUsesApplyPatchResources(t *testing.T) {
	workDir := t.TempDir()
	action, resources, err := permissionTarget(ToolCall{
		Name: "apply_patch",
		Args: map[string]any{"patchText": "*** Begin Patch\n*** Add File: docs/a.md\n+hello\n*** Update File: src/b.go\n*** Move to: src/c.go\n@@\n-old\n+new\n*** End Patch"},
		Tool: tool.NewApplyPatchTool(),
	}, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if action != "edit" {
		t.Fatalf("action = %q, want edit", action)
	}
	want := []string{"docs/a.md", "src/b.go", "src/c.go"}
	if len(resources) != len(want) {
		t.Fatalf("resources = %#v, want %#v", resources, want)
	}
	for i := range want {
		if resources[i] != want[i] {
			t.Fatalf("resources = %#v, want %#v", resources, want)
		}
	}
}

func TestCheckPermissionDenyApplyPatchByPath(t *testing.T) {
	workDir := t.TempDir()
	r := &runner{cfg: Config{
		WorkDir: workDir,
		Permissions: permission.Ruleset{
			{Action: "edit", Resource: "*", Effect: permission.EffectAllow},
			{Action: "edit", Resource: "src/*", Effect: permission.EffectDeny},
		},
	}}
	res, ok := r.checkPermission(ToolCall{
		ID:   "call_1",
		Name: "apply_patch",
		Args: map[string]any{"patchText": "*** Begin Patch\n*** Add File: src/main.go\n+package main\n*** End Patch"},
		Tool: tool.NewApplyPatchTool(),
	})
	if ok {
		t.Fatal("permission check allowed denied patch")
	}
	meta := permissionMetadata(t, res.Metadata)
	if meta["effect"] != "deny" || meta["action"] != "edit" || meta["resource"] != "src/main.go" {
		t.Fatalf("permission metadata = %#v", meta)
	}
}

func permissionMetadata(t *testing.T, metadata map[string]any) map[string]any {
	t.Helper()
	permissionValue, ok := metadata["permission"].(map[string]any)
	if !ok {
		t.Fatalf("missing permission metadata in %#v", metadata)
	}
	return permissionValue
}
