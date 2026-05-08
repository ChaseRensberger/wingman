package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

// TestOutputFormatMapsToOutputConfig answers: when Request.OutputSchema is
// set, does the wire request contain output_config.format.type == "json_schema"
// and preserve the schema?
func TestOutputFormatMapsToOutputConfig(t *testing.T) {
	c := &Client{model: "claude-test", maxTokens: defaultMaxTokens}
	req := models.Request{
		Messages: []models.Message{models.NewUserText("hi")},
		OutputSchema: &models.OutputSchema{
			Name: "test",
			Schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		},
	}
	built := c.buildRequest(req)
	wire := built.wire

	if wire.OutputConfig == nil {
		t.Fatal("expected OutputConfig in wire request")
	}
	if wire.OutputConfig.Format == nil {
		t.Fatal("expected OutputConfig.Format in wire request")
	}
	if wire.OutputConfig.Format.Type != "json_schema" {
		t.Errorf("Format.Type = %q, want json_schema", wire.OutputConfig.Format.Type)
	}
	if wire.OutputConfig.Format.Schema == nil {
		t.Fatal("expected Format.Schema in wire request")
	}

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal wire: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	oc, ok := m["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing or wrong type")
	}
	format, ok := oc["format"].(map[string]any)
	if !ok {
		t.Fatalf("format missing or wrong type")
	}
	if format["type"] != "json_schema" {
		t.Errorf("type = %v, want json_schema", format["type"])
	}
	schema, ok := format["schema"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing or wrong type")
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, want object", schema["type"])
	}
}
