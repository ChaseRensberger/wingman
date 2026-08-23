package server

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

func apiAgent(value *store.Agent) api.Agent {
	return api.Agent{
		ID: value.ID, Name: value.Name, Instructions: value.Instructions,
		Tools: slices.Clone(value.Tools), Permissions: slices.Clone(value.Permissions),
		ModelRef: value.ModelRef, Options: maps.Clone(value.Options), OutputSchema: maps.Clone(value.OutputSchema),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func storeAgent(value *api.AgentSpec) *store.Agent {
	return &store.Agent{
		ID: value.ID, Name: value.Name, Instructions: value.Instructions,
		Tools: slices.Clone(value.Tools), Permissions: slices.Clone(value.Permissions),
		ModelRef: value.ModelRef, Options: maps.Clone(value.Options), OutputSchema: maps.Clone(value.OutputSchema),
	}
}

func apiAgents(values []*store.Agent) []api.Agent {
	result := make([]api.Agent, len(values))
	for i, value := range values {
		result[i] = apiAgent(value)
	}
	return result
}

func apiClient(value *store.Client) api.Client {
	return api.Client{ID: value.ID, Name: value.Name, CreatedAt: value.CreatedAt}
}

func apiClients(values []*store.Client) []api.Client {
	result := make([]api.Client, len(values))
	for i, value := range values {
		result[i] = apiClient(value)
	}
	return result
}

func apiWorkspace(value *store.Workspace) api.Workspace {
	return api.Workspace{
		ID: value.ID, Name: value.Name, Path: value.Path, ClientID: value.ClientID,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func apiWorkspaces(values []*store.Workspace) []api.Workspace {
	result := make([]api.Workspace, len(values))
	for i, value := range values {
		result[i] = apiWorkspace(value)
	}
	return result
}

func apiSession(value *store.Session) api.Session {
	return api.Session{
		ID: value.ID, Title: value.Title, WorkDir: value.WorkDir, WorkspaceID: value.WorkspaceID,
		ClientID: value.ClientID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Version: value.AggregateVersion,
	}
}

func apiSessions(values []*store.Session) []api.Session {
	result := make([]api.Session, len(values))
	for i, value := range values {
		result[i] = apiSession(value)
	}
	return result
}

func apiSessionDetail(value *store.Session, history []models.Message, latest *store.ModelCall) api.SessionDetail {
	result := api.SessionDetail{Session: apiSession(value), History: history}
	if latest != nil {
		call := apiModelCall(*latest)
		result.LatestModelCall = &call
	}
	return result
}

func apiModelCall(value store.ModelCall) api.ModelCall {
	var cost *float64
	if value.Cost != nil {
		copy := *value.Cost
		cost = &copy
	}
	return api.ModelCall{
		ID: value.ID, SessionID: value.SessionID, RunID: value.RunID,
		AssistantMessageID: value.AssistantMessageID, Step: value.Step, Attempt: value.Attempt,
		Status: value.Status, AgentID: value.AgentID, ModelRef: value.ModelRef, Provider: value.Provider,
		ProviderRequestID: value.ProviderRequestID, API: value.API, ModelID: value.ModelID,
		FinishReason: value.FinishReason, StopReason: value.StopReason,
		ErrorType: value.ErrorType, ErrorMessage: value.ErrorMessage,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, ReasoningTokens: value.ReasoningTokens,
		CachedInputTokens: value.CachedInputTokens, CacheWriteTokens: value.CacheWriteTokens,
		TotalTokens: value.TotalTokens, ContextTokens: value.ContextTokens,
		ContextWindow: value.ContextWindow, ContextPercent: value.ContextPercent, Cost: cost,
		Trace: append(json.RawMessage(nil), value.Trace...), StartedAt: value.StartedAt, CompletedAt: value.CompletedAt,
	}
}

func apiModelCalls(values []store.ModelCall) []api.ModelCall {
	result := make([]api.ModelCall, len(values))
	for i, value := range values {
		result[i] = apiModelCall(value)
	}
	return result
}

func apiSessionRun(value store.SessionRun) api.SessionRun {
	sources := make([]api.InstructionSource, len(value.InstructionSources))
	for i, source := range value.InstructionSources {
		sources[i] = api.InstructionSource{
			Kind: source.Kind, Path: source.Path, SHA256: source.SHA256,
			ResolvedAt: source.ResolvedAt, Order: source.Order,
		}
	}
	return api.SessionRun{
		ID: value.ID, SessionID: value.SessionID, RequestID: value.RequestID,
		AdmittedVersion: value.AdmittedVersion, WorkDir: value.WorkDir, WorkspaceID: value.WorkspaceID,
		ClientID: value.ClientID, Sequence: value.Sequence, Status: value.Status, Message: value.Message,
		Agent: apiAgent(&value.Agent), ErrorType: value.ErrorType, ErrorMessage: value.ErrorMessage,
		EffectiveInstructions: value.EffectiveInstructions, InstructionSources: sources,
		CreatedAt: value.CreatedAt, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt, UpdatedAt: value.UpdatedAt,
	}
}

func apiSessionRuns(values []store.SessionRun) []api.SessionRun {
	result := make([]api.SessionRun, len(values))
	for i, value := range values {
		result[i] = apiSessionRun(value)
	}
	return result
}

func apiToolUse(value store.ToolUse) api.ToolUse {
	return api.ToolUse{
		ID: value.ID, SessionID: value.SessionID, RunID: value.RunID, ModelCallID: value.ModelCallID,
		AssistantMessageID: value.AssistantMessageID, PartID: value.PartID, Step: value.Step, Ordinal: value.Ordinal,
		CallID: value.CallID, Name: value.Name, Status: value.Status,
		Input: append(json.RawMessage(nil), value.InputJSON...), Output: value.Output,
		Structured: append(json.RawMessage(nil), value.StructuredJSON...), Metadata: append(json.RawMessage(nil), value.MetadataJSON...),
		ErrorType: value.ErrorType, ErrorMessage: value.ErrorMessage, ProposedAt: value.ProposedAt,
		AuthorizedAt: value.AuthorizedAt, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func apiToolUses(values []store.ToolUse) []api.ToolUse {
	result := make([]api.ToolUse, len(values))
	for i, value := range values {
		result[i] = apiToolUse(value)
	}
	return result
}

func apiPermissionRequest(value store.PermissionRequest) api.PermissionRequest {
	return api.PermissionRequest{
		ID: value.ID, SessionID: value.SessionID, RunID: value.RunID, ToolUseID: value.ToolUseID,
		CallID: value.CallID, Action: value.Action, Resources: slices.Clone(value.Resources), Status: value.Status,
		Response: value.Response, ErrorType: value.ErrorType, ErrorMessage: value.ErrorMessage,
		CreatedAt: value.CreatedAt, ResolvedAt: value.ResolvedAt, UpdatedAt: value.UpdatedAt,
	}
}

func apiPermissionRequests(values []store.PermissionRequest) []api.PermissionRequest {
	result := make([]api.PermissionRequest, len(values))
	for i, value := range values {
		result[i] = apiPermissionRequest(value)
	}
	return result
}

func apiPermissionGrant(value store.PermissionGrant) api.PermissionGrant {
	return api.PermissionGrant{ID: value.ID, SessionID: value.SessionID, Action: value.Action, Resource: value.Resource, CreatedAt: value.CreatedAt}
}

func apiPermissionGrants(values []store.PermissionGrant) []api.PermissionGrant {
	result := make([]api.PermissionGrant, len(values))
	for i, value := range values {
		result[i] = apiPermissionGrant(value)
	}
	return result
}

func apiSessionEvent(value store.SessionEvent) (api.SessionEvent, error) {
	schemaVersion := value.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = api.CurrentSessionEventSchemaVersion
	}
	raw := value.Data
	if len(raw) == 0 {
		raw = value.DataJSON
	}
	data, err := api.DecodeSessionEventDataForSchemaVersion(schemaVersion, api.SessionEventType(value.Type), raw)
	if err != nil {
		return api.SessionEvent{}, err
	}
	result := api.SessionEvent{ID: value.ID, SchemaVersion: schemaVersion, Type: api.SessionEventType(value.Type), Data: data}
	if !value.Time.IsZero() {
		result.Time = value.Time.UTC().Format(time.RFC3339Nano)
	}
	if value.Seq > 0 && value.SessionID != "" {
		result.Cursor = &api.SessionEventCursor{SessionID: value.SessionID, Seq: value.Seq}
	}
	return result, nil
}

func apiSessionEvents(values []store.SessionEvent) ([]api.SessionEvent, error) {
	result := make([]api.SessionEvent, len(values))
	for i, value := range values {
		event, err := apiSessionEvent(value)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", value.ID, err)
		}
		result[i] = event
	}
	return result, nil
}
