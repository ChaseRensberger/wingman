package models

import (
	"reflect"
	"testing"
)

func TestPartRoundTrip(t *testing.T) {
	cases := []Part{
		TextPart{Text: "hello"},
		TextPart{Text: "with sig", Signature: "openai-resp-id-1"},
		ReasoningPart{Reasoning: "let me think"},
		ReasoningPart{Reasoning: "", Signature: "anthropic-redacted-blob", Redacted: true},
		ImagePart{Data: "iVBORw0KGgo=", MimeType: "image/png"},
		ToolCallPart{
			CallID: "call_1",
			Name:   "bash",
			Input:  map[string]any{"cmd": "ls", "timeout": 30.0},
		},
		ToolResultPart{
			CallID: "call_1",
			Output: []Part{
				TextPart{Text: "ok"},
				ImagePart{Data: "abc", MimeType: "image/png"},
			},
		},
		ToolResultPart{
			CallID:  "call_2",
			Output:  []Part{TextPart{Text: "permission denied"}},
			IsError: true,
		},
	}
	for _, in := range cases {
		raw, err := MarshalPart(in)
		if err != nil {
			t.Fatalf("marshal %T: %v", in, err)
		}
		out, err := UnmarshalPart(raw)
		if err != nil {
			t.Fatalf("unmarshal %T (%s): %v", in, raw, err)
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("%T: in=%#v out=%#v raw=%s", in, in, out, raw)
		}
	}
}

func TestUnknownPartType(t *testing.T) {
	// Unknown discriminators round-trip as OpaquePart so storage
	// preserves plugin-defined parts even when the plugin isn't
	// loaded.
	in := []byte(`{"type":"banana","peel":true}`)
	p, err := UnmarshalPart(in)
	if err != nil {
		t.Fatalf("expected OpaquePart, got error: %v", err)
	}
	op, ok := p.(OpaquePart)
	if !ok {
		t.Fatalf("expected OpaquePart, got %T", p)
	}
	if op.TypeName != "banana" {
		t.Errorf("TypeName: got %q want %q", op.TypeName, "banana")
	}
	out, err := MarshalPart(op)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("round-trip changed bytes:\n in=%s\nout=%s", in, out)
	}
}

func TestMissingPartType(t *testing.T) {
	if _, err := UnmarshalPart([]byte(`{}`)); err == nil {
		t.Fatal("expected error for missing type discriminator")
	}
}

func TestContentRoundTrip(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: Content{
			TextPart{Text: "calling tool"},
			ToolCallPart{CallID: "c1", Name: "bash", Input: map[string]any{"cmd": "ls"}},
		},
	}
	raw, err := msg.Content.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got Content
	if err := got.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(msg.Content, got) {
		t.Errorf("in=%#v out=%#v raw=%s", msg.Content, got, raw)
	}
}
