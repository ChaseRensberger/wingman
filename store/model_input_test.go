package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestRecordedModelInputPersistence(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ctx := t.Context()
			var data store.Store = memory.NewStore()
			path := filepath.Join(t.TempDir(), "input.db")
			if backend == "sqlite" {
				var err error
				data, err = store.NewSQLiteStore(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { _ = data.Close() })
			if err := data.CreateSession(&store.Session{ID: "ses_input"}); err != nil {
				t.Fatal(err)
			}
			hash, blobs, err := models.SnapshotCallInput(models.Request{System: "Original instructions", Messages: []models.Message{models.NewUserText("exact old input")}})
			if err != nil {
				t.Fatal(err)
			}
			trace, _ := json.Marshal(models.CallTrace{Version: "1", InputHash: hash})
			call := store.ModelCall{ID: "mcl_input", SessionID: "ses_input", Step: 1, Status: store.ModelCallStatusStarted, Trace: trace, MetadataJSON: trace, InputBlobs: blobs}
			if err := data.UpsertModelCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			call.InputBlobs = nil
			call.Status = store.ModelCallStatusAborted
			if err := data.UpsertModelCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			for hash := range blobs {
				blobs[hash][0] = 'x'
			}
			if err := data.(interface {
				RebuildSessionProjections(context.Context, string) error
			}).RebuildSessionProjections(ctx, "ses_input"); err != nil {
				t.Fatal(err)
			}
			if backend == "sqlite" {
				if err := data.Close(); err != nil {
					t.Fatal(err)
				}
				data, err = store.NewSQLiteStore(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			input, err := data.GetModelCallInput(ctx, "ses_input", "mcl_input")
			if err != nil || input == nil || input.System != "Original instructions" || input.Messages[0].Content[0].(models.TextPart).Text != "exact old input" {
				t.Fatalf("input = %#v, %v", input, err)
			}
			if _, err := data.GetModelCallInput(ctx, "other-session", "mcl_input"); !errors.Is(err, store.ErrModelCallNotFound) {
				t.Fatalf("cross-session lookup: %v", err)
			}
			call.Trace, call.MetadataJSON = nil, nil
			if err := data.UpsertModelCall(ctx, call); err == nil {
				t.Fatal("allowed overwriting historical input reference")
			}
			if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_old", SessionID: "ses_input", Step: 2}); err != nil {
				t.Fatal(err)
			}
			if input, err := data.GetModelCallInput(ctx, "ses_input", "mcl_old"); err != nil || input != nil {
				t.Fatalf("old call input = %#v, %v", input, err)
			}
			calls, err := data.ListModelCalls(ctx, "ses_input")
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range calls {
				if len(call.InputBlobs) != 0 {
					t.Fatal("full input leaked into call list")
				}
			}
			session, err := data.GetSession("ses_input")
			if err != nil {
				t.Fatal(err)
			}
			if err := data.PurgeSession(ctx, session.ID, session.AggregateVersion); err != nil {
				t.Fatal(err)
			}
			if _, err := data.GetModelCallInput(ctx, "ses_input", "mcl_input"); !errors.Is(err, store.ErrModelCallNotFound) {
				t.Fatalf("deleted call lookup: %v", err)
			}
		})
	}
}
