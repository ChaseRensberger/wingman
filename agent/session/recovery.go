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
	"github.com/chaserensberger/wingman/tool"
)

// ErrRecoveryBlocked means that automatic continuation cannot safely proceed.
var ErrRecoveryBlocked = errors.New("run recovery blocked")

func (s *Session) prepareRecovery(ctx context.Context, tools []tool.Tool) (*run.Recovery, bool, error) {
	if s.store == nil || s.runID == "" {
		return nil, false, fmt.Errorf("%w: a stored run is required", ErrRecoveryBlocked)
	}
	admitted, err := s.store.GetSessionRun(ctx, s.id, s.runID)
	if err != nil {
		return nil, false, err
	}
	if admitted.Status != store.SessionRunStatusRunning {
		return nil, false, fmt.Errorf("%w: the run is not running", ErrRecoveryBlocked)
	}
	messages, err := s.store.ListMessages(ctx, s.id)
	if err != nil {
		return nil, false, err
	}
	userSaved := false
	recovery := &run.Recovery{Tools: map[string]run.RecoveredTool{}, InterruptedMessageIDs: map[string]bool{}}
	for _, message := range messages {
		if message.RunID == s.runID && message.Role == string(models.RoleUser) {
			userSaved = true
		}
		if message.RunID == s.runID && message.Role == string(models.RoleAssistant) && message.State == string(models.MessageStateFailed) {
			recovery.InterruptedMessageIDs[message.ID] = true
		}
	}
	calls, err := s.store.ListModelCalls(ctx, s.id)
	if err != nil {
		return nil, false, err
	}
	var latest *store.ModelCall
	for _, call := range calls {
		if call.RunID != s.runID {
			continue
		}
		if call.Status == store.ModelCallStatusCompleted {
			delete(recovery.InterruptedMessageIDs, call.AssistantMessageID)
		}
		recovery.Usage.InputTokens += call.InputTokens
		recovery.Usage.OutputTokens += call.OutputTokens
		recovery.Usage.TotalTokens += call.TotalTokens
		recovery.Usage.ReasoningTokens += call.ReasoningTokens
		recovery.Usage.CachedInputTokens += call.CachedInputTokens
		recovery.Usage.CacheWriteTokens += call.CacheWriteTokens
		if latest == nil || call.Step > latest.Step || (call.Step == latest.Step && call.Attempt > latest.Attempt) {
			copy := call
			latest = &copy
		}
	}
	if latest == nil {
		return recovery, userSaved, nil
	}
	if !userSaved {
		return nil, false, fmt.Errorf("%w: saved input is missing", ErrRecoveryBlocked)
	}
	var assistant *models.Message
	for _, message := range s.history {
		if message.ID == latest.AssistantMessageID {
			copy := message
			assistant = &copy
			break
		}
	}
	if assistant == nil {
		return nil, false, fmt.Errorf("%w: assistant checkpoint is missing", ErrRecoveryBlocked)
	}
	var trace models.CallTrace
	if len(latest.MetadataJSON) > 0 {
		if err := json.Unmarshal(latest.MetadataJSON, &trace); err != nil {
			return nil, false, err
		}
	}
	if latest.Status != store.ModelCallStatusCompleted {
		interrupted := latest.Status == store.ModelCallStatusAborted && (latest.ErrorType == "process_interrupted" || latest.ErrorType == "canceled")
		retryScheduled := latest.Status == store.ModelCallStatusFailed && trace.Retry != nil && trace.Retry.Decision == "scheduled"
		if !interrupted && !retryScheduled {
			return nil, false, fmt.Errorf("%w: model call failed: %s", ErrRecoveryBlocked, latest.ErrorMessage)
		}
		for _, part := range assistant.Content {
			if p, ok := part.(models.ToolPart); ok && p.ProviderExecuted {
				return nil, false, fmt.Errorf("%w: provider tool outcome is uncertain", ErrRecoveryBlocked)
			}
		}
		recovery.Step, recovery.AttemptOffset = latest.Step-1, latest.Attempt
		recovery.InterruptedMessageIDs[assistant.ID] = true
		return recovery, userSaved, nil
	}
	uses, err := s.store.ListToolUses(ctx, s.id)
	if err != nil {
		return nil, false, err
	}
	registry, err := tool.Compose(tools)
	if err != nil {
		return nil, false, err
	}
	// Inspect the whole batch before resetting or executing any interrupted call.
	for _, use := range uses {
		if use.RunID != s.runID || use.Status != store.ToolUseStatusInterrupted {
			continue
		}
		t, lookupErr := registry.Get(use.Name)
		if lookupErr != nil || (!use.StartedAt.IsZero() && (!use.ReplaySafe || !t.Definition().ReplaySafe)) {
			return nil, false, fmt.Errorf("%w: tool %s (%s) has an uncertain outcome or is unavailable", ErrRecoveryBlocked, use.Name, use.ID)
		}
	}
	for _, use := range uses {
		if use.RunID != s.runID || use.ModelCallID != latest.ID {
			continue
		}
		saved := run.RecoveredTool{ToolUseID: use.ID, ProposedAt: use.ProposedAt, AuthorizedAt: use.AuthorizedAt}
		if err := json.Unmarshal(use.InputJSON, &saved.Args); err != nil {
			return nil, false, err
		}
		if use.Status == store.ToolUseStatusInterrupted {
			use.Status = store.ToolUseStatusAuthorized
			if use.AuthorizedAt.IsZero() {
				use.Status = store.ToolUseStatusProposed
			}
			// Keep evidence of execution if another crash occurs before replay starts.
			use.CompletedAt = time.Time{}
			use.ErrorType, use.ErrorMessage = "", ""
			use.UpdatedAt = time.Now().UTC()
			if err := s.store.SaveToolUse(ctx, use); err != nil {
				return nil, false, err
			}
		} else if use.Status == store.ToolUseStatusCompleted || use.Status == store.ToolUseStatusFailed || use.Status == store.ToolUseStatusDeclined {
			result := run.ToolResult{CallID: use.CallID, ToolUseID: use.ID, Name: use.Name, Args: saved.Args, Output: use.Output, Error: use.ErrorMessage, ErrorType: use.ErrorType, IsError: use.Status != store.ToolUseStatusCompleted, Status: run.ToolUseStatus(use.Status)}
			if !use.StartedAt.IsZero() {
				result.Duration = use.CompletedAt.Sub(use.StartedAt)
			}
			for _, value := range []struct {
				data   []byte
				target any
			}{{use.StructuredJSON, &result.Structured}, {use.MetadataJSON, &result.Metadata}, {use.OutputPartsJSON, &result.OutputParts}} {
				if len(value.data) > 0 {
					if err := json.Unmarshal(value.data, value.target); err != nil {
						return nil, false, err
					}
				}
			}
			saved.Result = &result
		}
		recovery.Tools[use.PartID] = saved
	}
	recovery.Step = latest.Step
	recovery.Turn = &run.Turn{Step: latest.Step, Attempt: latest.Attempt, ModelCallID: latest.ID, ProviderRequestID: latest.ProviderRequestID, Assistant: *assistant, StartedAt: latest.StartedAt, CompletedAt: latest.CompletedAt}
	if assistant.Usage != nil {
		recovery.Turn.Usage = *assistant.Usage
	}
	recovery.Turn.Trace = trace
	return recovery, userSaved, nil
}
