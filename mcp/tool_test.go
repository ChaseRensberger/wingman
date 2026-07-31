package mcp

import (
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDefinitionPreservesOutputSchema(t *testing.T) {
	adapted := &mcpTool{server: "docs", remoteName: "search", def: &mcpsdk.Tool{
		Name: "search", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"},
	}}
	definition := adapted.Definition()
	if definition.OutputSchema["type"] != "object" {
		t.Fatalf("output schema = %#v", definition.OutputSchema)
	}
}
