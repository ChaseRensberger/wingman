package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerLoadsAndExecutesPluginTool(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := `{
  "id": "test.echo",
  "name": "Echo",
  "command": [` + quoteJSON(os.Args[0]) + `, "-test.run=TestPluginHostHelper", "--", "plugin-helper"],
  "tools": [{
    "name": "echo",
    "description": "Echo text",
    "input_schema": {
      "type": "object",
      "properties": {"text": {"type": "string", "description": "Text to echo"}},
      "required": ["text"]
    }
  }]
}`
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	mgr, err := New(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	statuses, errs := mgr.Status()
	if len(errs) != 0 {
		t.Fatalf("unexpected load errors: %#v", errs)
	}
	if len(statuses) != 1 || statuses[0].ID != "test.echo" || !statuses[0].Running {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}

	tools := mgr.Tools()
	if len(tools) != 1 || tools[0].Name() != "echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}
	out, err := tools[0].Execute(context.Background(), map[string]any{"text": "hello"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestPluginHostHelper(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "plugin-helper" {
		return
	}
	defer os.Exit(0)

	var req rpcRequest
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		os.Exit(1)
	}
	var params toolExecuteParams
	if data, err := json.Marshal(req.Params); err == nil {
		_ = json.Unmarshal(data, &params)
	}
	text, _ := params.Params["text"].(string)
	res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	res.Result, _ = json.Marshal(toolExecuteResult{Text: text})
	_ = json.NewEncoder(os.Stdout).Encode(res)
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
