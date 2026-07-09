package run

import (
	"testing"

	"github.com/chaserensberger/wingman/permission"
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
	action, resources := permissionTarget(ToolCall{Name: "write", Args: map[string]any{"filePath": "docs/index.md"}}, "/repo")
	if action != "edit" || len(resources) != 1 || resources[0] != "docs/index.md" {
		t.Fatalf("target = %s %#v", action, resources)
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
