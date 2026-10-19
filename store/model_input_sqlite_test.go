package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestModelInputBlobsDeduplicateAndCommitWithCall(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "input.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if err := data.CreateSession(&Session{ID: "ses_input"}); err != nil {
		t.Fatal(err)
	}
	hash, blobs, err := models.SnapshotCallInput(models.Request{System: "instructions", Messages: []models.Message{models.NewUserText("message")}})
	if err != nil {
		t.Fatal(err)
	}
	trace, _ := json.Marshal(models.CallTrace{Version: "1", InputHash: hash})
	call := ModelCall{ID: "first", SessionID: "ses_input", Trace: trace, MetadataJSON: trace, InputBlobs: blobs}
	if _, err := data.db.Exec(`CREATE TRIGGER fail_call_event BEFORE INSERT ON aggregate_events WHEN NEW.event_type = 'session.model_call.saved' BEGIN SELECT RAISE(ABORT, 'cannot save event'); END`); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(t.Context(), call); err == nil {
		t.Fatal("expected event write to fail")
	}
	var count int
	if err := data.db.QueryRow(`SELECT COUNT(*) FROM model_input_blobs`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("uncommitted blobs: %d, %v", count, err)
	}
	if _, err := data.db.Exec(`DROP TRIGGER fail_call_event`); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(t.Context(), call); err != nil {
		t.Fatal(err)
	}
	call.ID = "retry"
	if err := data.UpsertModelCall(t.Context(), call); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow(`SELECT COUNT(*) FROM model_input_blobs`).Scan(&count); err != nil || count != len(blobs) {
		t.Fatalf("blob count = %d, want %d: %v", count, len(blobs), err)
	}
	session, err := data.GetSession("ses_input")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.PurgeSession(t.Context(), session.ID, session.AggregateVersion); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow(`SELECT COUNT(*) FROM model_input_blobs`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned blobs = %d, %v", count, err)
	}
}
