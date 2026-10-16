package session

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestBoundedToolOutputSurvivesHydration(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			workDir := t.TempDir()
			full := strings.Repeat("result\n", 20000)
			client := &sequencedToolClient{messages: []models.Message{
				{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "large", Input: map[string]any{}}}},
				{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}},
			}}
			sess := New(WithID("ses_history"), WithStore(data), WithWorkDir(workDir), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("large", "large", tool.Definition{Name: "large", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) { return tool.Result{Text: full}, nil })))
			t.Cleanup(func() { _ = sess.Close(context.Background()) })
			if _, err := sess.Run(t.Context(), "go"); err != nil {
				t.Fatal(err)
			}
			reopened := New(WithID("ses_history"), WithStore(data), WithWorkDir(workDir))
			t.Cleanup(func() { _ = reopened.Close(context.Background()) })
			if err := reopened.hydrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			part := reopened.History()[1].Content[0].(models.ToolPart)
			if len(part.Output) > tool.MaxOutputBytes+512 || part.Metadata["truncated"] != true {
				t.Fatalf("size=%d metadata=%v", len(part.Output), part.Metadata)
			}
			bytes, err := os.ReadFile(part.Metadata["output_path"].(string))
			if err != nil || string(bytes) != full {
				t.Fatalf("full output: %v", err)
			}
			page, err := tool.NewReadTool().Execute(t.Context(), tool.Invocation{WorkDir: workDir, SessionID: "ses_history", Input: map[string]any{"filePath": part.Metadata["output_ref"], "offset": 19000, "limit": 1}})
			if err != nil || !strings.Contains(page.Text, "19000: result") {
				t.Fatalf("page=%v err=%v", page, err)
			}
		})
	}
}
