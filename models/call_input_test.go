package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCallInputFreezesContentAndReusesUnchangedBlobs(t *testing.T) {
	req := Request{
		System:       "Original instructions",
		Messages:     []Message{{ID: "msg_one", Revision: 2, Role: RoleUser, Content: Content{TextPart{Text: "original"}}}},
		Tools:        []ToolDef{{Name: "read", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice:   ToolChoiceAuto,
		OutputSchema: &OutputSchema{Name: "answer", Strict: true, Schema: map[string]any{"type": "object"}},
		Model:        ModelRef{BaseURL: "https://private-endpoint"},
	}
	hash, blobs, err := SnapshotCallInput(req)
	if err != nil {
		t.Fatal(err)
	}
	read := func(key string) (json.RawMessage, error) { return blobs[key], nil }
	before, err := ResolveCallInput(hash, read)
	if err != nil {
		t.Fatal(err)
	}
	req.System = "New instructions"
	req.Messages[0].Content[0] = TextPart{Text: "changed"}
	req.Tools[0].InputSchema["type"] = "string"
	req.OutputSchema.Schema["type"] = "string"
	after, err := ResolveCallInput(hash, read)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("snapshot changed: %#v, %v", after, err)
	}
	if before.Messages[0].ID != "msg_one" || before.Messages[0].Revision != 2 || before.Tools[0].InputSchema["type"] != "object" || !before.OutputSchema.Strict {
		t.Fatalf("lost input details: %#v", before)
	}
	for _, data := range blobs {
		if strings.Contains(string(data), "private-endpoint") {
			t.Fatal("stored route configuration with model input")
		}
	}
	_, next, err := SnapshotCallInput(Request{System: before.System, Messages: append(before.Messages, NewUserText("next")), Tools: before.Tools, ToolChoice: before.ToolChoice, OutputSchema: before.OutputSchema})
	if err != nil {
		t.Fatal(err)
	}
	newBlobs := 0
	for key := range next {
		if _, ok := blobs[key]; !ok {
			newBlobs++
		}
	}
	if newBlobs != 2 {
		t.Fatalf("new blobs = %d; want only a new message and manifest", newBlobs)
	}
	for key := range blobs {
		blobs[key] = json.RawMessage(`{}`)
		break
	}
	if _, err := ResolveCallInput(hash, read); err == nil {
		t.Fatal("accepted corrupted snapshot")
	}
}
