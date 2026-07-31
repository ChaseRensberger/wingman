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

// persistMessage atomically writes a complete message revision to the store.
func (s *Session) persistMessage(ctx context.Context, msg models.Message, idx int) (models.Message, error) {
	if s.store == nil {
		return msg, nil
	}
	if msg.ID == "" {
		msg.ID = store.NewID(store.PrefixMessage)
	}
	if msg.Revision == 0 {
		msg.Revision = 1
	}
	if msg.State == "" {
		msg.State = models.MessageStateCompleted
	}
	now := time.Now().UTC()
	sm := store.StoredMessage{
		ID:        msg.ID,
		SessionID: s.id,
		Idx:       idx,
		Role:      string(msg.Role),
		Revision:  msg.Revision,
		State:     string(msg.State),
		CreatedAt: now,
		UpdatedAt: now,
	}
	metadata, err := marshalMessageMetadata(msg)
	if err != nil {
		return models.Message{}, err
	}
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return models.Message{}, fmt.Errorf("marshal metadata: %w", err)
		}
		sm.MetadataJSON = b
	}
	for i, part := range msg.Content {
		if models.PartID(part) == "" {
			part = models.WithPartID(part, store.NewID(store.PrefixPart))
			msg.Content[i] = part
		}
		payload, err := models.MarshalPart(part)
		if err != nil {
			return models.Message{}, fmt.Errorf("marshal part[%d]: %w", i, err)
		}
		sm.Parts = append(sm.Parts, store.StoredPart{
			ID:          models.PartID(part),
			MessageID:   sm.ID,
			Sequence:    i,
			Kind:        part.Type(),
			PayloadJSON: payload,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	if err := s.store.SaveMessage(ctx, sm); err != nil {
		return models.Message{}, fmt.Errorf("save message: %w", err)
	}
	return msg, nil
}

type modelCallRecorder struct {
	store     store.Store
	sessionID string
	runID     string
	agentID   string
	model     models.ModelRef
	modelInfo models.ModelInfo
}

// toolUseRecorder durably records the lifecycle of one model-proposed tool
// use. Its fields are immutable, so one recorder can safely serve parallel
// tool calls within a run.
type toolUseRecorder struct {
	store     store.Store
	sessionID string
	runID     string
}

func (r *toolUseRecorder) Propose(ctx context.Context, info run.ToolUseProposeInfo) (string, error) {
	input, err := json.Marshal(info.Args)
	if err != nil {
		return "", fmt.Errorf("marshal tool use input: %w", err)
	}
	use := toolUseRecord(r.sessionID, r.runID, info.Step, info.Ordinal, info.CallID, info.Name, info.MessageID, info.PartID, info.ModelCallID)
	use.ID = store.NewID(store.PrefixToolUse)
	use.Status = store.ToolUseStatusProposed
	use.InputJSON = input
	use.ProposedAt = info.ProposedAt
	if err := r.store.SaveToolUse(ctx, use); err != nil {
		return "", err
	}
	return use.ID, nil
}

func (r *toolUseRecorder) Authorize(ctx context.Context, info run.ToolUseAuthorizeInfo) error {
	input, err := json.Marshal(info.Args)
	if err != nil {
		return fmt.Errorf("marshal tool use input: %w", err)
	}
	use := toolUseRecord(r.sessionID, r.runID, info.Step, info.Ordinal, info.CallID, info.Name, info.MessageID, info.PartID, info.ModelCallID)
	use.ID = info.ToolUseID
	use.Status = store.ToolUseStatusAuthorized
	use.InputJSON = input
	use.AuthorizedAt = info.AuthorizedAt
	return r.store.SaveToolUse(ctx, use)
}

func (r *toolUseRecorder) Start(ctx context.Context, info run.ToolUseStartInfo) error {
	input, err := json.Marshal(info.Args)
	if err != nil {
		return fmt.Errorf("marshal tool use input: %w", err)
	}
	use := toolUseRecord(r.sessionID, r.runID, info.Step, info.Ordinal, info.CallID, info.Name, info.MessageID, info.PartID, info.ModelCallID)
	use.ID = info.ToolUseID
	use.Status = store.ToolUseStatusStarted
	use.InputJSON = input
	use.StartedAt = info.StartedAt
	return r.store.SaveToolUse(ctx, use)
}

func (r *toolUseRecorder) Finish(ctx context.Context, info run.ToolUseFinishInfo) error {
	input, err := json.Marshal(info.Args)
	if err != nil {
		return fmt.Errorf("marshal tool use input: %w", err)
	}
	metadata, err := json.Marshal(info.ToolResult.Metadata)
	if err != nil {
		return fmt.Errorf("marshal tool use metadata: %w", err)
	}
	use := toolUseRecord(r.sessionID, r.runID, info.Step, info.Ordinal, info.CallID, info.Name, info.MessageID, info.PartID, info.ModelCallID)
	use.ID = info.ToolUseID
	use.Status = storeToolUseStatus(info.Status)
	use.InputJSON = input
	use.Output = info.ToolResult.Output
	use.MetadataJSON = metadata
	use.ErrorType = info.ErrorType
	use.ErrorMessage = info.ErrorMessage
	if use.ErrorMessage == "" && info.Failure != nil {
		use.ErrorMessage = info.Failure.Error()
	}
	use.ProposedAt = info.ProposedAt
	use.AuthorizedAt = info.AuthorizedAt
	use.StartedAt = info.StartedAt
	use.CompletedAt = info.CompletedAt
	return r.store.SaveToolUse(ctx, use)
}

func toolUseRecord(sessionID, runID string, step, ordinal int, callID, name, messageID, partID, modelCallID string) store.ToolUse {
	return store.ToolUse{
		SessionID:          sessionID,
		RunID:              runID,
		ModelCallID:        modelCallID,
		AssistantMessageID: messageID,
		PartID:             partID,
		Step:               step,
		Ordinal:            ordinal,
		CallID:             callID,
		Name:               name,
	}
}

func storeToolUseStatus(status run.ToolUseStatus) string {
	switch status {
	case run.ToolUseStatusCompleted:
		return store.ToolUseStatusCompleted
	case run.ToolUseStatusFailed:
		return store.ToolUseStatusFailed
	case run.ToolUseStatusInterrupted:
		return store.ToolUseStatusInterrupted
	case run.ToolUseStatusDeclined:
		return store.ToolUseStatusDeclined
	default:
		return string(status)
	}
}

func (r *modelCallRecorder) Start(ctx context.Context, info run.ModelCallStartInfo) (string, error) {
	call := modelCallRecord(r.sessionID, r.runID, r.agentID, r.model, r.modelInfo, run.Turn{
		ModelCallID: store.NewID(store.PrefixModelCall),
		Step:        info.Step,
		Attempt:     info.Attempt,
		StartedAt:   info.StartedAt,
		Trace:       info.Trace,
	})
	call.Status = store.ModelCallStatusStarted
	call.AssistantMessageID = info.MessageID
	if err := r.store.UpsertModelCall(ctx, call); err != nil {
		return "", err
	}
	return call.ID, nil
}

func (r *modelCallRecorder) Finish(ctx context.Context, info run.ModelCallFinishInfo) error {
	turn := run.Turn{
		ModelCallID:       info.CallID,
		Step:              info.Step,
		Attempt:           info.Attempt,
		ProviderRequestID: info.ProviderRequestID,
		Usage:             info.Usage,
		StartedAt:         info.StartedAt,
		CompletedAt:       info.CompletedAt,
		Trace:             info.Trace,
		Failure:           info.Failure,
	}
	if info.Assistant != nil {
		turn.Assistant = *info.Assistant
	}
	return r.store.UpsertModelCall(ctx, modelCallRecord(r.sessionID, r.runID, r.agentID, r.model, r.modelInfo, turn))
}

func (s *Session) persistModelCall(ctx context.Context, msgID string, turn run.Turn, model models.ModelRef, info models.ModelInfo, runID, agentID, stopReason string, structuredOutput map[string]any) error {
	if s.store == nil {
		return nil
	}
	if turn.ModelCallID == "" {
		turn.ModelCallID = store.NewID(store.PrefixModelCall)
	}
	call := modelCallRecord(s.id, runID, agentID, model, info, turn)
	call.AssistantMessageID = msgID
	call.StopReason = stopReason
	if structuredOutput != nil {
		encoded, err := json.Marshal(structuredOutput)
		if err != nil {
			return fmt.Errorf("marshal model call structured output: %w", err)
		}
		call.StructuredOutputJSON = encoded
	}
	return s.store.UpsertModelCall(ctx, call)
}

func modelCallRecord(sessionID, runID, agentID string, model models.ModelRef, info models.ModelInfo, turn run.Turn) store.ModelCall {
	now := time.Now().UTC()
	usage := turn.Usage
	if usage.Empty() && turn.Assistant.Usage != nil {
		usage = *turn.Assistant.Usage
	}
	call := store.ModelCall{
		ID:                turn.ModelCallID,
		SessionID:         sessionID,
		RunID:             runID,
		Step:              turn.Step,
		Attempt:           turn.Attempt,
		Status:            store.ModelCallStatusCompleted,
		AgentID:           agentID,
		ModelRef:          model.Ref(),
		Provider:          model.Provider,
		ProviderRequestID: turn.ProviderRequestID,
		API:               string(model.API),
		ModelID:           model.ID,
		FinishReason:      string(turn.Assistant.FinishReason),
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		ReasoningTokens:   usage.ReasoningTokens,
		CachedInputTokens: usage.CachedInputTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
		TotalTokens:       usage.TotalOrComputed(),
		ContextTokens:     usage.ContextTokens(),
		ContextWindow:     info.ContextWindow,
		ContextPercent:    usage.ContextPercent(info.ContextWindow),
		Cost:              estimatedCost(usage, info),
		StartedAt:         turn.StartedAt.UTC(),
		CompletedAt:       turn.CompletedAt.UTC(),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if call.Attempt == 0 {
		call.Attempt = 1
	}
	if turn.Failure != nil {
		call.ErrorMessage = turn.Failure.Error()
		if errors.Is(turn.Failure, context.Canceled) || errors.Is(turn.Failure, context.DeadlineExceeded) {
			call.Status = store.ModelCallStatusAborted
			if errors.Is(turn.Failure, context.DeadlineExceeded) {
				call.ErrorType = "deadline_exceeded"
			} else {
				call.ErrorType = "canceled"
			}
		} else {
			call.Status = store.ModelCallStatusFailed
			call.ErrorType = "model_error"
		}
	}
	if turn.Assistant.Origin != nil {
		call.Provider = turn.Assistant.Origin.Provider
		call.API = string(turn.Assistant.Origin.API)
		call.ModelID = turn.Assistant.Origin.ModelID
	}
	if turn.Trace.Version != "" {
		b, err := json.Marshal(turn.Trace)
		if err == nil {
			call.MetadataJSON = b
			call.Trace = b
		}
	}
	return call
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
		ID:       sm.ID,
		Revision: sm.Revision,
		State:    models.MessageState(sm.State),
		Role:     models.Role(sm.Role),
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
		if models.PartID(part) == "" {
			part = models.WithPartID(part, sp.ID)
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
