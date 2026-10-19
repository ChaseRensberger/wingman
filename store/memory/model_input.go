package memory

import (
	"context"
	"encoding/json"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

// GetModelCallInput returns frozen effective input, or nil if the call predates recording.
func (s *Store) GetModelCallInput(ctx context.Context, sessionID, callID string) (*models.CallInput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	call := s.modelCalls[callID]
	if call == nil || call.SessionID != sessionID {
		return nil, store.ErrModelCallNotFound
	}
	hash := store.ModelCallInputHash(*call)
	if hash == "" {
		return nil, nil
	}
	return models.ResolveCallInput(hash, func(key string) (json.RawMessage, error) {
		return s.modelInputBlobs[sessionID][key], nil
	})
}
