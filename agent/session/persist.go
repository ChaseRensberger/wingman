package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

// WithStore injects a store.Store for message persistence. Nil means no
// persistence (in-memory only).
func WithStore(st store.Store) Option {
	return func(s *Session) { s.store = st }
}

// WithID sets the session identifier. Used by the server when resuming
// an existing session; bare New() mints a fresh ID.
func WithID(id string) Option {
	return func(s *Session) { s.id = id }
}

// hydrate loads prior history from the store when the session has a
// store and its in-memory history is empty.
func (s *Session) hydrate(ctx context.Context) error {
	if s.store == nil || len(s.history) > 0 {
		return nil
	}
	storedMsgs, err := s.store.ListMessages(ctx, s.id)
	if err != nil {
		if err == store.ErrSessionNotFound {
			return nil
		}
		return fmt.Errorf("hydrate: %w", err)
	}
	msgs := make([]models.Message, len(storedMsgs))
	for i, sm := range storedMsgs {
		m, err := storedMessageToModel(sm)
		if err != nil {
			return fmt.Errorf("hydrate message[%d]: %w", i, err)
		}
		msgs[i] = m
	}
	s.history = msgs
	return nil
}

// persistMessage writes a single message and its parts to the store.
func (s *Session) persistMessage(ctx context.Context, msg models.Message, idx int) error {
	if s.store == nil {
		return nil
	}
	now := time.Now().UTC()
	sm := store.StoredMessage{
		ID:        store.NewID(store.PrefixMessage),
		SessionID: s.id,
		Idx:       idx,
		Role:      string(msg.Role),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(msg.Metadata) > 0 {
		b, err := json.Marshal(msg.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		sm.MetadataJSON = b
	}
	if err := s.store.UpsertMessage(ctx, sm); err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	for i, part := range msg.Content {
		payload, err := models.MarshalPart(part)
		if err != nil {
			return fmt.Errorf("marshal part[%d]: %w", i, err)
		}
		sp := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   sm.ID,
			Sequence:    i,
			Kind:        part.Type(),
			PayloadJSON: payload,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.store.UpsertPart(ctx, sp); err != nil {
			return fmt.Errorf("upsert part[%d]: %w", i, err)
		}
	}
	return nil
}

func storedMessageToModel(sm store.StoredMessage) (models.Message, error) {
	msg := models.Message{
		Role: models.Role(sm.Role),
	}
	if len(sm.MetadataJSON) > 0 {
		var meta models.Meta
		if err := json.Unmarshal(sm.MetadataJSON, &meta); err != nil {
			return models.Message{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
		msg.Metadata = meta
	}
	content := make(models.Content, len(sm.Parts))
	for i, sp := range sm.Parts {
		part, err := models.UnmarshalPart(sp.PayloadJSON)
		if err != nil {
			return models.Message{}, fmt.Errorf("unmarshal part[%d]: %w", i, err)
		}
		content[i] = part
	}
	msg.Content = content
	return msg, nil
}
