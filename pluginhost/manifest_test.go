package pluginhost

import (
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/tool"
)

func TestValidateManifestRejectsDuplicateToolNames(t *testing.T) {
	spec := ToolSpec{Name: "search", Description: "Search", InputSchema: map[string]any{"type": "object"}}
	err := validateToolSpecs([]ToolSpec{spec, spec})
	if err == nil || !strings.Contains(err.Error(), "duplicate tool name") {
		t.Fatalf("validateManifest() error = %v", err)
	}
}

func TestRPCDefinitionCarriesExecutionContract(t *testing.T) {
	spec := ToolSpec{
		Name: "search", Description: "Search", InputSchema: map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"}, Sequential: true, DirectoryScoped: true,
		Permission: &tool.PermissionTarget{Action: "search", ResourceFields: []string{"query"}},
	}
	definition := (&rpcTool{spec: spec}).Definition()
	if definition.RawInputSchema["type"] != "object" || definition.OutputSchema["type"] != "object" || !definition.Sequential || !definition.DirectoryScoped || definition.Permission.Action != "search" {
		t.Fatalf("definition = %#v", definition)
	}
}
