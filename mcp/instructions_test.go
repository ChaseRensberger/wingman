package mcp

import (
	"context"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

type instructedConnection struct {
	fakeConnection
	text string
}

func (c *instructedConnection) InitializeResult() *mcpsdk.InitializeResult {
	return &mcpsdk.InitializeResult{Instructions: c.text}
}

func TestInstructionsFollowToolGeneration(t *testing.T) {
	text := "original instructions"
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		return &instructedConnection{text: text}, testTools("tool"), nil
	})
	defer m.Close()
	if err := m.Connect(t.Context(), "server"); err != nil {
		t.Fatal(err)
	}
	oldTools := m.Tools()
	text = "replacement instructions"
	if err := m.Connect(t.Context(), "server"); err != nil {
		t.Fatal(err)
	}
	old, current := Instructions(oldTools), Instructions(m.Tools())
	if len(old) != 1 || old[0].Text != "original instructions" || len(current) != 1 || current[0].Text != text {
		t.Fatalf("old=%v current=%v", old, current)
	}
	if len(Instructions(nil)) != 0 {
		t.Fatal("instructions without selected tools")
	}
}
