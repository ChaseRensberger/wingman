package server

import (
	"encoding/json"
	"maps"
	"slices"

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
