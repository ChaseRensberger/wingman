package agent

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestSchemaForGeneratesAnthropicCompatibleSchema answers: does the
// reflection helper produce JSON Schema with additionalProperties:false,
// every field in required, and no $ref usage?
func TestSchemaForGeneratesAnthropicCompatibleSchema(t *testing.T) {
	type Contact struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	got := SchemaFor[Contact]()
	if got == nil {
		t.Fatal("SchemaFor returned nil")
	}
	if got.Schema["additionalProperties"] != false {
		t.Errorf("expected additionalProperties:false, got %v", got.Schema["additionalProperties"])
	}
	required, _ := got.Schema["required"].([]any)
	if len(required) != 2 {
		t.Errorf("expected 2 required fields, got %v", required)
	}
	b, _ := json.Marshal(got.Schema)
	if bytes.Contains(b, []byte("$ref")) {
		t.Errorf("schema contains $ref: %s", string(b))
	}
}
