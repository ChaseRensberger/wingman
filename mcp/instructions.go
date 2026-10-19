package mcp

import (
	"github.com/chaserensberger/wingman/tool"
	"sort"
)

// Instruction contains usage guidance from one connected MCP server.
type Instruction struct{ Server, Text string }

// Instructions returns guidance from the connection generations backing tools.
func Instructions(tools []tool.Tool) []Instruction {
	byServer := map[string]string{}
	for _, t := range tools {
		if remote, ok := t.(*mcpTool); ok && remote.generation.instructions != "" {
			byServer[remote.server] = remote.generation.instructions
		}
	}
	out := make([]Instruction, 0, len(byServer))
	for name, text := range byServer {
		out = append(out, Instruction{Server: name, Text: text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Server < out[j].Server })
	return out
}
