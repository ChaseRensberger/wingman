package api

import (
	"encoding/json"
	"testing"
)

func TestDecodeSessionEventDataUsesDiscriminator(t *testing.T) {
	data, err := DecodeSessionEventData(SessionEventTextDelta, json.RawMessage(`{"run_id":"run_1","delta":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	delta, ok := data.(*ContentDeltaEventData)
	if !ok {
		t.Fatalf("payload type = %T", data)
	}
	if delta.RunID != "run_1" || delta.Delta != "hello" {
		t.Fatalf("payload = %#v", delta)
	}
}

func TestUnknownSessionEventDataRoundTripsVerbatim(t *testing.T) {
	raw := json.RawMessage(`{"plugin":"value","count":2}`)
	data, err := DecodeSessionEventData("plugin.custom", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := data.(UnknownSessionEventData); !ok {
		t.Fatalf("payload type = %T", data)
	}
	encoded, err := json.Marshal(SessionEvent{ID: "evt_1", Type: "plugin.custom", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"id":"evt_1","schema_version":1,"type":"plugin.custom","data":{"plugin":"value","count":2}}` {
		t.Fatalf("event = %s", encoded)
	}
}

func TestSessionEventKnownPayloadMarshalsWithoutWrapper(t *testing.T) {
	event := SessionEvent{
		ID: "evt_1", Type: SessionEventRunCompleted,
		Cursor: &SessionEventCursor{SessionID: "ses_1", Seq: 4},
		Data:   &RunEventData{RunID: "run_1", Status: "completed", Steps: 2},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"id":"evt_1","schema_version":1,"type":"session.run.completed","cursor":{"session_id":"ses_1","seq":4},"data":{"run_id":"run_1","status":"completed","steps":2}}` {
		t.Fatalf("event = %s", encoded)
	}
}

func TestSessionEventUnmarshalDecodesV1AndPreservesUnknownType(t *testing.T) {
	var event SessionEvent
	if err := json.Unmarshal([]byte(`{"id":"evt_1","schema_version":1,"type":"plugin.custom","data":{"plugin":"value"}}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.SchemaVersion != SessionEventSchemaVersionV1 {
		t.Fatalf("schema version = %d", event.SchemaVersion)
	}
	if _, ok := event.Data.(UnknownSessionEventData); !ok {
		t.Fatalf("payload type = %T", event.Data)
	}
}

func TestSessionEventUnmarshalRejectsUnsupportedSchemaVersion(t *testing.T) {
	var event SessionEvent
	err := json.Unmarshal([]byte(`{"id":"evt_1","schema_version":2,"type":"session.text.delta","data":{"run_id":"run_1","delta":"hello"}}`), &event)
	if err == nil || err.Error() != "unsupported session event schema version 2" {
		t.Fatalf("unmarshal error = %v", err)
	}
}
