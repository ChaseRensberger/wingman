package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chaserensberger/wingman/models"
)

// ModelCallInputHash returns the immutable input reference recorded in a call's trace.
func ModelCallInputHash(call ModelCall) string {
	data := call.Trace
	if len(data) == 0 {
		data = call.MetadataJSON
	}
	var trace struct {
		InputHash string `json:"input_hash"`
	}
	_ = json.Unmarshal(data, &trace)
	return trace.InputHash
}

// ValidateModelCallInput checks content hashes and prevents replacing a call's input.
func ValidateModelCallInput(call, previous ModelCall) error {
	hash, old := ModelCallInputHash(call), ModelCallInputHash(previous)
	if previous.ID != "" && hash != old {
		return fmt.Errorf("model call input is immutable")
	}
	if hash == "" && len(call.InputBlobs) == 0 {
		return nil
	}
	if hash == old && len(call.InputBlobs) == 0 {
		return nil
	}
	for key, data := range call.InputBlobs {
		if fmt.Sprintf("%x", sha256.Sum256(data)) != key {
			return fmt.Errorf("invalid model input blob %s", key)
		}
	}
	_, err := models.ResolveCallInput(hash, func(key string) (json.RawMessage, error) {
		return call.InputBlobs[key], nil
	})
	return err
}

// GetModelCallInput returns frozen effective input, or nil if the call predates recording.
func (s *SQLiteStore) GetModelCallInput(ctx context.Context, sessionID, callID string) (*models.CallInput, error) {
	var metadata []byte
	err := s.db.QueryRowContext(ctx, `SELECT metadata_json FROM model_calls WHERE session_id = ? AND id = ?`, sessionID, callID).Scan(&metadata)
	if err == sql.ErrNoRows {
		return nil, ErrModelCallNotFound
	}
	if err != nil {
		return nil, err
	}
	hash := ModelCallInputHash(ModelCall{MetadataJSON: metadata})
	if hash == "" {
		return nil, nil
	}
	return models.ResolveCallInput(hash, func(key string) (json.RawMessage, error) {
		var data []byte
		err := s.db.QueryRowContext(ctx, `SELECT content FROM model_input_blobs WHERE session_id = ? AND hash = ?`, sessionID, key).Scan(&data)
		return data, err
	})
}
