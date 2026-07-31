package pluginhost

import (
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/tool"
)

func TestValidateManifestRejectsDuplicateToolNames(t *testing.T) {
	spec := ToolSpec{Name: "search", Description: "Search", InputSchema: tool.InputSchema{Type: "object"}}
	err := validateManifest(Manifest{ID: "example", Command: []string{"example"}, Tools: []ToolSpec{spec, spec}})
	if err == nil || !strings.Contains(err.Error(), "duplicate tool name") {
		t.Fatalf("validateManifest() error = %v", err)
	}
}

func TestRPCDefinitionCarriesExecutionContract(t *testing.T) {
	spec := ToolSpec{
		Name: "search", Description: "Search", InputSchema: tool.InputSchema{Type: "object"},
		OutputSchema: map[string]any{"type": "object"}, Sequential: true, DirectoryScoped: true,
		Permission: &tool.PermissionTarget{Action: "search", ResourceFields: []string{"query"}},
	}
	definition := (&rpcTool{spec: spec}).Definition()
	if definition.OutputSchema["type"] != "object" || !definition.Sequential || !definition.DirectoryScoped || definition.Permission.Action != "search" {
		t.Fatalf("definition = %#v", definition)
	}
}
