package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
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
	calls, err := s.store.ListModelCalls(ctx, s.id)
	if err != nil {
		return fmt.Errorf("hydrate model calls: %w", err)
	}
	callsByMessageID := make(map[string]store.ModelCall, len(calls))
	for _, call := range calls {
		if call.AssistantMessageID != "" {
			callsByMessageID[call.AssistantMessageID] = call
		}
	}
	msgs := make([]models.Message, len(storedMsgs))
	for i, sm := range storedMsgs {
		m, err := storedMessageToModel(sm)
		if err != nil {
			return fmt.Errorf("hydrate message[%d]: %w", i, err)
		}
		if call, ok := callsByMessageID[sm.ID]; ok {
			ApplyModelCall(&m, call)
		}
		msgs[i] = m
	}
	s.history = models.NormalizeMessages(msgs)
	return nil
}

// persistMessage writes a single message and its parts to the store.
func (s *Session) persistMessage(ctx context.Context, msg models.Message, idx int) (string, error) {
	if s.store == nil {
		return "", nil
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
	metadata, err := marshalMessageMetadata(msg)
	if err != nil {
		return "", err
	}
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return "", fmt.Errorf("marshal metadata: %w", err)
		}
		sm.MetadataJSON = b
	}
	if err := s.store.UpsertMessage(ctx, sm); err != nil {
		return "", fmt.Errorf("upsert message: %w", err)
	}
	for i, part := range msg.Content {
		payload, err := models.MarshalPart(part)
		if err != nil {
			return "", fmt.Errorf("marshal part[%d]: %w", i, err)
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
			return "", fmt.Errorf("upsert part[%d]: %w", i, err)
		}
	}
	return sm.ID, nil
}

func (s *Session) persistModelCall(ctx context.Context, msgID string, turn run.Turn, model models.ModelRef, info models.ModelInfo, stopReason string) error {
	if s.store == nil {
		return nil
	}
	now := time.Now().UTC()
	msg := turn.Assistant
	usage := models.Usage{}
	if msg.Usage != nil {
		usage = *msg.Usage
	}
	call := store.ModelCall{
		ID:                 store.NewID(store.PrefixModelCall),
		SessionID:          s.id,
		AssistantMessageID: msgID,
		Step:               turn.Step,
		Attempt:            1,
		Status:             store.ModelCallStatusCompleted,
		AgentID:            s.agentID,
		ModelRef:           model.Ref(),
		Provider:           model.Provider,
		API:                string(model.API),
		ModelID:            model.ID,
		FinishReason:       string(msg.FinishReason),
		StopReason:         stopReason,
		InputTokens:        usage.InputTokens,
		OutputTokens:       usage.OutputTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		CachedInputTokens:  usage.CachedInputTokens,
		CacheWriteTokens:   usage.CacheWriteTokens,
		TotalTokens:        usage.TotalOrComputed(),
		ContextTokens:      usage.ContextTokens(),
		ContextWindow:      info.ContextWindow,
		ContextPercent:     usage.ContextPercent(info.ContextWindow),
		Cost:               estimatedCost(usage, info),
		StartedAt:          turn.StartedAt.UTC(),
		CompletedAt:        turn.CompletedAt.UTC(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if turn.Failure != nil {
		call.ErrorMessage = turn.Failure.Error()
		if errors.Is(turn.Failure, context.Canceled) || errors.Is(turn.Failure, context.DeadlineExceeded) {
			call.Status = store.ModelCallStatusAborted
		} else {
			call.Status = store.ModelCallStatusFailed
		}
	}
	if msg.Origin != nil {
		call.Provider = msg.Origin.Provider
		call.API = string(msg.Origin.API)
		call.ModelID = msg.Origin.ModelID
	}
	if turn.Trace.Version != "" {
		b, err := json.Marshal(turn.Trace)
		if err == nil {
			call.MetadataJSON = b
			call.Trace = b
		}
	}
	return s.store.UpsertModelCall(ctx, call)
}

func estimatedCost(usage models.Usage, info models.ModelInfo) *float64 {
	if usage.Empty() || (info.InputCostPerMTok == 0 && info.OutputCostPerMTok == 0) {
		return nil
	}
	cost := float64(usage.InputTokens)/1_000_000*info.InputCostPerMTok +
		float64(usage.OutputTokens)/1_000_000*info.OutputCostPerMTok
	return &cost
}

func StoredMessageToModel(sm store.StoredMessage) (models.Message, error) {
	msg := models.Message{
		Role: models.Role(sm.Role),
	}
	if len(sm.MetadataJSON) > 0 {
		var meta models.Meta
		if err := json.Unmarshal(sm.MetadataJSON, &meta); err != nil {
			return models.Message{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
		if err := unmarshalMessageMetadata(meta, &msg); err != nil {
			return models.Message{}, err
		}
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

func storedMessageToModel(sm store.StoredMessage) (models.Message, error) {
	return StoredMessageToModel(sm)
}

func marshalMessageMetadata(msg models.Message) (models.Meta, error) {
	meta := models.Meta{}
	for k, v := range msg.Metadata {
		meta[k] = v
	}
	return meta, nil
}

func unmarshalMessageMetadata(meta models.Meta, msg *models.Message) error {
	if len(meta) > 0 {
		msg.Metadata = meta
	}
	return nil
}

func ApplyModelCall(msg *models.Message, call store.ModelCall) {
	usage := models.Usage{
		InputTokens:       call.InputTokens,
		OutputTokens:      call.OutputTokens,
		TotalTokens:       call.TotalTokens,
		ReasoningTokens:   call.ReasoningTokens,
		CachedInputTokens: call.CachedInputTokens,
		CacheWriteTokens:  call.CacheWriteTokens,
	}
	if !usage.Empty() {
		msg.Usage = &usage
	}
	if call.FinishReason != "" {
		msg.FinishReason = models.FinishReason(call.FinishReason)
	}
	if call.Provider != "" || call.API != "" || call.ModelID != "" {
		msg.Origin = &models.MessageOrigin{
			Provider: call.Provider,
			API:      models.API(call.API),
			ModelID:  call.ModelID,
		}
	}
}
