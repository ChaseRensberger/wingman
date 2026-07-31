package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chaserensberger/wingman/tool"
)

type mcpTool struct {
	generation *connectionGeneration
	server     string
	remoteName string
	def        *mcpsdk.Tool
}

func (t *mcpTool) Name() string { return toolName(t.server, t.remoteName) }

func (t *mcpTool) Description() string { return t.def.Description }

func (t *mcpTool) Definition() tool.Definition {
	return tool.Definition{
		Name:           t.Name(),
		Description:    t.Description(),
		InputSchema:    tool.InputSchema{Type: "object"},
		RawInputSchema: schemaMap(t.def.InputSchema),
		OutputSchema:   schemaMap(t.def.OutputSchema),
	}
}

func (t *mcpTool) Execute(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	res, err := t.generation.callTool(ctx, t.remoteName, inv.Input)
	if err != nil {
		return tool.Result{}, err
	}
	text := resultText(res)
	if res.IsError {
		if text == "" {
			text = "MCP tool returned an error"
		}
		return tool.Result{Metadata: resultMetadata(t.server, t.remoteName, res)}, fmt.Errorf("%s", text)
	}
	if text == "" && res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			text = string(b)
		}
	}
	return tool.Result{Text: text, Structured: res.StructuredContent, Metadata: resultMetadata(t.server, t.remoteName, res)}, nil
}

func resultText(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, content := range res.Content {
		switch item := content.(type) {
		case *mcpsdk.TextContent:
			if strings.TrimSpace(item.Text) != "" {
				parts = append(parts, item.Text)
			}
		default:
			if b, err := json.Marshal(item); err == nil && len(b) > 0 {
				parts = append(parts, string(b))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func resultMetadata(server, remoteName string, res *mcpsdk.CallToolResult) map[string]any {
	meta := map[string]any{
		"source":      "mcp",
		"server":      server,
		"remote_name": remoteName,
	}
	if res != nil && res.StructuredContent != nil {
		meta["structured_content"] = res.StructuredContent
	}
	return meta
}
