package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// AggregateType identifies a durable consistency and ordering boundary.
type AggregateType string

const (
	AggregateSession AggregateType = "session"
)

const (
	EventSessionCreated        = "session.created"
	EventSessionRenamed        = "session.renamed"
	EventSessionMoved          = "session.moved"
	EventSessionRunAdmitted    = "session.run.admitted"
	EventSessionRunStarted     = "session.run.started"
	EventSessionRunCompleted   = "session.run.completed"
	EventSessionRunFailed      = "session.run.failed"
	EventSessionRunAborted     = "session.run.aborted"
	EventSessionMessageSaved   = "session.message.saved"
	EventSessionModelCallSaved = "session.model_call.saved"
	EventSessionToolUseSaved   = "session.tool_use.saved"
)

var ErrAggregateVersionConflict = errors.New("aggregate version conflict")

// AggregateVersionConflict reports an optimistic concurrency failure.
type AggregateVersionConflict struct {
	Aggregate AggregateRef
	Expected  int64
	Actual    int64
}

func (e *AggregateVersionConflict) Error() string {
	return fmt.Sprintf("%s %s: %v: expected %d, actual %d", e.Aggregate.Type, e.Aggregate.ID, ErrAggregateVersionConflict, e.Expected, e.Actual)
}

func (e *AggregateVersionConflict) Unwrap() error { return ErrAggregateVersionConflict }

// AggregateRef identifies one aggregate stream.
type AggregateRef struct {
	Type AggregateType `json:"type"`
	ID   string        `json:"id"`
}

// AggregateEvent is one immutable fact in an aggregate stream.
type AggregateEvent struct {
	GlobalSequence int64           `json:"global_sequence"`
	ID             string          `json:"id"`
	Aggregate      AggregateRef    `json:"aggregate"`
	Version        int64           `json:"version"`
	Type           string          `json:"type"`
	SchemaVersion  int             `json:"schema_version"`
	Time           time.Time       `json:"time"`
	Data           json.RawMessage `json:"data"`
	CausationID    string          `json:"causation_id,omitempty"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	ClientID       string          `json:"client_id,omitempty"`
	RunID          string          `json:"run_id,omitempty"`
}

type sessionCreatedData struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	WorkDir     string `json:"work_dir,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type sessionRenamedData struct {
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

type sessionMovedData struct {
	WorkDir     string `json:"work_dir,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

type sessionRunAdmittedData struct {
	Run json.RawMessage `json:"run"`
}

type sessionRunTransitionData struct {
	Run json.RawMessage `json:"run"`
}

type sessionMessageSavedData struct {
	Message storedMessageSnapshot `json:"message"`
}

type sessionModelCallSavedData struct {
	Call modelCallSnapshot `json:"call"`
}

type sessionToolUseSavedData struct {
	ToolUse toolUseSnapshot `json:"tool_use"`
}

// []byte deliberately keeps these payloads opaque: the store does not impose
// JSON validity or rewrite their bytes while serializing aggregate history.
type toolUseSnapshot struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	RunID              string    `json:"run_id,omitempty"`
	ModelCallID        string    `json:"model_call_id,omitempty"`
	AssistantMessageID string    `json:"assistant_message_id,omitempty"`
	PartID             string    `json:"part_id,omitempty"`
	Step               int       `json:"step"`
	Ordinal            int       `json:"ordinal"`
	CallID             string    `json:"call_id,omitempty"`
	Name               string    `json:"name"`
	Status             string    `json:"status"`
	InputJSON          []byte    `json:"input_json,omitempty"`
	Output             string    `json:"output,omitempty"`
	StructuredJSON     []byte    `json:"structured_json,omitempty"`
	MetadataJSON       []byte    `json:"metadata_json,omitempty"`
	ErrorType          string    `json:"error_type,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	ProposedAt         time.Time `json:"proposed_at"`
	AuthorizedAt       time.Time `json:"authorized_at,omitempty"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type modelCallSnapshot struct {
	ID                   string          `json:"id"`
	SessionID            string          `json:"session_id"`
	RunID                string          `json:"run_id,omitempty"`
	AssistantMessageID   string          `json:"assistant_message_id,omitempty"`
	Step                 int             `json:"step"`
	Attempt              int             `json:"attempt"`
	Status               string          `json:"status"`
	AgentID              string          `json:"agent_id,omitempty"`
	ModelRef             string          `json:"model_ref,omitempty"`
	Provider             string          `json:"provider,omitempty"`
	ProviderRequestID    string          `json:"provider_request_id,omitempty"`
	API                  string          `json:"api,omitempty"`
	ModelID              string          `json:"model_id,omitempty"`
	FinishReason         string          `json:"finish_reason,omitempty"`
	StopReason           string          `json:"stop_reason,omitempty"`
	ErrorType            string          `json:"error_type,omitempty"`
	ErrorMessage         string          `json:"error_message,omitempty"`
	InputTokens          int             `json:"input_tokens"`
	OutputTokens         int             `json:"output_tokens"`
	ReasoningTokens      int             `json:"reasoning_tokens,omitempty"`
	CachedInputTokens    int             `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens     int             `json:"cache_write_tokens,omitempty"`
	TotalTokens          int             `json:"total_tokens"`
	ContextTokens        int             `json:"context_tokens"`
	ContextWindow        int             `json:"context_window,omitempty"`
	ContextPercent       float64         `json:"context_percent,omitempty"`
	Cost                 *float64        `json:"cost,omitempty"`
	StructuredOutputJSON json.RawMessage `json:"structured_output_json,omitempty"`
	MetadataJSON         json.RawMessage `json:"metadata_json,omitempty"`
	Trace                json.RawMessage `json:"trace,omitempty"`
	StartedAt            time.Time       `json:"started_at"`
	CompletedAt          time.Time       `json:"completed_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type storedMessageSnapshot struct {
	ID           string               `json:"id"`
	SessionID    string               `json:"session_id"`
	RunID        string               `json:"run_id,omitempty"`
	Idx          int                  `json:"idx"`
	Role         string               `json:"role"`
	Revision     int64                `json:"revision"`
	State        string               `json:"state"`
	MetadataJSON json.RawMessage      `json:"metadata_json,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Parts        []storedPartSnapshot `json:"parts"`
}

type storedPartSnapshot struct {
	ID          string          `json:"id"`
	MessageID   string          `json:"message_id"`
	Sequence    int             `json:"sequence"`
	Kind        string          `json:"kind"`
	PayloadJSON json.RawMessage `json:"payload_json"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// NewSessionRunAdmittedEvent records an immutable admitted run projection.
func NewSessionRunAdmittedEvent(run SessionRun) (AggregateEvent, error) {
	runData, err := marshalSessionRun(run)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal admitted run projection: %w", err)
	}
	data, err := json.Marshal(sessionRunAdmittedData{Run: runData})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.run.admitted: %w", err)
	}
	return AggregateEvent{
		ID:            NewID(PrefixEvent),
		Aggregate:     AggregateRef{Type: AggregateSession, ID: run.SessionID},
		Type:          EventSessionRunAdmitted,
		SchemaVersion: 1,
		Time:          run.CreatedAt,
		Data:          data,
		ClientID:      run.ClientID,
		RunID:         run.ID,
	}, nil
}

// ProjectSessionRunAdmission decodes the immutable run projection in an admission event.
func ProjectSessionRunAdmission(event AggregateEvent) (SessionRun, error) {
	if event.Type != EventSessionRunAdmitted || event.SchemaVersion != 1 {
		return SessionRun{}, fmt.Errorf("project session run: unsupported event %q schema version %d", event.Type, event.SchemaVersion)
	}
	var data sessionRunAdmittedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode session.run.admitted: %w", err)
	}
	run, err := unmarshalSessionRun(data.Run)
	if err != nil {
		return SessionRun{}, err
	}
	if run.SessionID != event.Aggregate.ID {
		return SessionRun{}, fmt.Errorf("project session run: payload session %q does not match aggregate", run.SessionID)
	}
	if run.AdmittedVersion != event.Version {
		return SessionRun{}, fmt.Errorf("project session run %s: payload admitted version %d does not match event version %d", run.ID, run.AdmittedVersion, event.Version)
	}
	return run, nil
}

// NewSessionRunTransitionEvent records an immutable run lifecycle transition.
func NewSessionRunTransitionEvent(run SessionRun) (AggregateEvent, error) {
	typeName, ok := map[string]string{
		SessionRunStatusRunning:   EventSessionRunStarted,
		SessionRunStatusCompleted: EventSessionRunCompleted,
		SessionRunStatusFailed:    EventSessionRunFailed,
		SessionRunStatusAborted:   EventSessionRunAborted,
	}[run.Status]
	if !ok {
		return AggregateEvent{}, fmt.Errorf("session run %s: unsupported transition status %q", run.ID, run.Status)
	}
	runData, err := marshalSessionRun(run)
	if err != nil {
		return AggregateEvent{}, err
	}
	data, err := json.Marshal(sessionRunTransitionData{Run: runData})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal %s: %w", typeName, err)
	}
	return AggregateEvent{
		ID:            NewID(PrefixEvent),
		Aggregate:     AggregateRef{Type: AggregateSession, ID: run.SessionID},
		Type:          typeName,
		SchemaVersion: 1,
		Time:          run.UpdatedAt,
		Data:          data,
		ClientID:      run.ClientID,
		RunID:         run.ID,
	}, nil
}

// ProjectSessionRunTransition decodes an immutable run transition snapshot.
func ProjectSessionRunTransition(event AggregateEvent) (SessionRun, error) {
	if event.SchemaVersion != 1 {
		return SessionRun{}, fmt.Errorf("project session run: unsupported event %q schema version %d", event.Type, event.SchemaVersion)
	}
	var data sessionRunTransitionData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode %s: %w", event.Type, err)
	}
	run, err := unmarshalSessionRun(data.Run)
	if err != nil {
		return SessionRun{}, err
	}
	if run.SessionID != event.Aggregate.ID || run.ID != event.RunID {
		return SessionRun{}, fmt.Errorf("project session run: transition payload does not match aggregate event")
	}
	wantType, ok := map[string]string{
		SessionRunStatusRunning:   EventSessionRunStarted,
		SessionRunStatusCompleted: EventSessionRunCompleted,
		SessionRunStatusFailed:    EventSessionRunFailed,
		SessionRunStatusAborted:   EventSessionRunAborted,
	}[run.Status]
	if !ok || event.Type != wantType {
		return SessionRun{}, fmt.Errorf("project session run %s: event %q does not match status %q", run.ID, event.Type, run.Status)
	}
	return run, nil
}

// NewSessionMessageSavedEvent records one complete authoritative message revision.
func NewSessionMessageSavedEvent(message StoredMessage) (AggregateEvent, error) {
	snapshot := storedMessageSnapshot{
		ID: message.ID, SessionID: message.SessionID, RunID: message.RunID,
		Idx: message.Idx, Role: message.Role, Revision: message.Revision, State: message.State,
		MetadataJSON: message.MetadataJSON, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
		Parts: make([]storedPartSnapshot, len(message.Parts)),
	}
	for i, part := range message.Parts {
		snapshot.Parts[i] = storedPartSnapshot{ID: part.ID, MessageID: part.MessageID, Sequence: part.Sequence, Kind: part.Kind, PayloadJSON: part.PayloadJSON, CreatedAt: part.CreatedAt, UpdatedAt: part.UpdatedAt}
	}
	data, err := json.Marshal(sessionMessageSavedData{Message: snapshot})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.message.saved: %w", err)
	}
	return AggregateEvent{ID: NewID(PrefixEvent), Aggregate: AggregateRef{Type: AggregateSession, ID: message.SessionID}, Type: EventSessionMessageSaved, SchemaVersion: 1, Time: message.UpdatedAt, Data: data, RunID: message.RunID}, nil
}

// ProjectSessionMessageSaved decodes an immutable message revision snapshot.
func ProjectSessionMessageSaved(event AggregateEvent) (StoredMessage, error) {
	if event.Type != EventSessionMessageSaved || event.SchemaVersion != 1 {
		return StoredMessage{}, fmt.Errorf("project session message: unsupported event %q schema version %d", event.Type, event.SchemaVersion)
	}
	var data sessionMessageSavedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return StoredMessage{}, fmt.Errorf("project session message: decode: %w", err)
	}
	message := StoredMessage{ID: data.Message.ID, SessionID: data.Message.SessionID, RunID: data.Message.RunID, Idx: data.Message.Idx, Role: data.Message.Role, Revision: data.Message.Revision, State: data.Message.State, MetadataJSON: data.Message.MetadataJSON, CreatedAt: data.Message.CreatedAt, UpdatedAt: data.Message.UpdatedAt, Parts: make([]StoredPart, len(data.Message.Parts))}
	if message.ID == "" || message.SessionID != event.Aggregate.ID || message.RunID != event.RunID || message.Revision < 1 {
		return StoredMessage{}, fmt.Errorf("project session message: snapshot does not match aggregate event")
	}
	partIDs := make(map[string]struct{}, len(data.Message.Parts))
	sequences := make(map[int]struct{}, len(data.Message.Parts))
	for i, part := range data.Message.Parts {
		if part.ID == "" || part.MessageID != message.ID {
			return StoredMessage{}, fmt.Errorf("project session message %s: invalid part snapshot", message.ID)
		}
		if _, exists := partIDs[part.ID]; exists {
			return StoredMessage{}, fmt.Errorf("project session message %s: duplicate part %s", message.ID, part.ID)
		}
		if _, exists := sequences[part.Sequence]; exists {
			return StoredMessage{}, fmt.Errorf("project session message %s: duplicate part sequence %d", message.ID, part.Sequence)
		}
		partIDs[part.ID] = struct{}{}
		sequences[part.Sequence] = struct{}{}
		message.Parts[i] = StoredPart{ID: part.ID, MessageID: part.MessageID, Sequence: part.Sequence, Kind: part.Kind, PayloadJSON: part.PayloadJSON, CreatedAt: part.CreatedAt, UpdatedAt: part.UpdatedAt}
	}
	return message, nil
}

// NewSessionModelCallSavedEvent records one authoritative model-call snapshot.
func NewSessionModelCallSavedEvent(call ModelCall) (AggregateEvent, error) {
	snapshot := modelCallSnapshot{
		ID: call.ID, SessionID: call.SessionID, RunID: call.RunID, AssistantMessageID: call.AssistantMessageID,
		Step: call.Step, Attempt: call.Attempt, Status: call.Status, AgentID: call.AgentID, ModelRef: call.ModelRef,
		Provider: call.Provider, ProviderRequestID: call.ProviderRequestID, API: call.API, ModelID: call.ModelID,
		FinishReason: call.FinishReason, StopReason: call.StopReason, ErrorType: call.ErrorType, ErrorMessage: call.ErrorMessage,
		InputTokens: call.InputTokens, OutputTokens: call.OutputTokens, ReasoningTokens: call.ReasoningTokens,
		CachedInputTokens: call.CachedInputTokens, CacheWriteTokens: call.CacheWriteTokens, TotalTokens: call.TotalTokens,
		ContextTokens: call.ContextTokens, ContextWindow: call.ContextWindow, ContextPercent: call.ContextPercent, Cost: call.Cost,
		StructuredOutputJSON: call.StructuredOutputJSON, MetadataJSON: call.MetadataJSON, Trace: call.Trace,
		StartedAt: call.StartedAt, CompletedAt: call.CompletedAt, CreatedAt: call.CreatedAt, UpdatedAt: call.UpdatedAt,
	}
	data, err := json.Marshal(sessionModelCallSavedData{Call: snapshot})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.model_call.saved: %w", err)
	}
	return AggregateEvent{ID: NewID(PrefixEvent), Aggregate: AggregateRef{Type: AggregateSession, ID: call.SessionID}, Type: EventSessionModelCallSaved, SchemaVersion: 1, Time: call.UpdatedAt, Data: data, RunID: call.RunID}, nil
}

// ProjectSessionModelCallSaved decodes one authoritative model-call snapshot.
func ProjectSessionModelCallSaved(event AggregateEvent) (ModelCall, error) {
	if event.Type != EventSessionModelCallSaved || event.SchemaVersion != 1 {
		return ModelCall{}, fmt.Errorf("project session model call: unsupported event %q schema version %d", event.Type, event.SchemaVersion)
	}
	var data sessionModelCallSavedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return ModelCall{}, fmt.Errorf("project session model call: decode: %w", err)
	}
	call := ModelCall{ID: data.Call.ID, SessionID: data.Call.SessionID, RunID: data.Call.RunID, AssistantMessageID: data.Call.AssistantMessageID, Step: data.Call.Step, Attempt: data.Call.Attempt, Status: data.Call.Status, AgentID: data.Call.AgentID, ModelRef: data.Call.ModelRef, Provider: data.Call.Provider, ProviderRequestID: data.Call.ProviderRequestID, API: data.Call.API, ModelID: data.Call.ModelID, FinishReason: data.Call.FinishReason, StopReason: data.Call.StopReason, ErrorType: data.Call.ErrorType, ErrorMessage: data.Call.ErrorMessage, InputTokens: data.Call.InputTokens, OutputTokens: data.Call.OutputTokens, ReasoningTokens: data.Call.ReasoningTokens, CachedInputTokens: data.Call.CachedInputTokens, CacheWriteTokens: data.Call.CacheWriteTokens, TotalTokens: data.Call.TotalTokens, ContextTokens: data.Call.ContextTokens, ContextWindow: data.Call.ContextWindow, ContextPercent: data.Call.ContextPercent, Cost: data.Call.Cost, StructuredOutputJSON: data.Call.StructuredOutputJSON, MetadataJSON: data.Call.MetadataJSON, Trace: data.Call.Trace, StartedAt: data.Call.StartedAt, CompletedAt: data.Call.CompletedAt, CreatedAt: data.Call.CreatedAt, UpdatedAt: data.Call.UpdatedAt}
	if call.ID == "" || call.SessionID != event.Aggregate.ID || call.RunID != event.RunID || call.Attempt < 1 || call.StartedAt.IsZero() || call.CreatedAt.IsZero() || call.UpdatedAt.IsZero() {
		return ModelCall{}, fmt.Errorf("project session model call: snapshot does not match aggregate event")
	}
	return call, nil
}

// NewSessionToolUseSavedEvent records one authoritative tool-use lifecycle snapshot.
func NewSessionToolUseSavedEvent(use ToolUse) (AggregateEvent, error) {
	snapshot := toolUseSnapshot{
		ID: use.ID, SessionID: use.SessionID, RunID: use.RunID, ModelCallID: use.ModelCallID,
		AssistantMessageID: use.AssistantMessageID, PartID: use.PartID, Step: use.Step, Ordinal: use.Ordinal,
		CallID: use.CallID, Name: use.Name, Status: use.Status, InputJSON: use.InputJSON, Output: use.Output,
		StructuredJSON: use.StructuredJSON, MetadataJSON: use.MetadataJSON, ErrorType: use.ErrorType,
		ErrorMessage: use.ErrorMessage, ProposedAt: use.ProposedAt, AuthorizedAt: use.AuthorizedAt,
		StartedAt: use.StartedAt, CompletedAt: use.CompletedAt, CreatedAt: use.CreatedAt, UpdatedAt: use.UpdatedAt,
	}
	data, err := json.Marshal(sessionToolUseSavedData{ToolUse: snapshot})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.tool_use.saved: %w", err)
	}
	return AggregateEvent{ID: NewID(PrefixEvent), Aggregate: AggregateRef{Type: AggregateSession, ID: use.SessionID}, Type: EventSessionToolUseSaved, SchemaVersion: 1, Time: use.UpdatedAt, Data: data, RunID: use.RunID}, nil
}

// ProjectSessionToolUseSaved decodes one authoritative tool-use lifecycle snapshot.
func ProjectSessionToolUseSaved(event AggregateEvent) (ToolUse, error) {
	if event.Type != EventSessionToolUseSaved || event.SchemaVersion != 1 {
		return ToolUse{}, fmt.Errorf("project session tool use: unsupported event %q schema version %d", event.Type, event.SchemaVersion)
	}
	var data sessionToolUseSavedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return ToolUse{}, fmt.Errorf("project session tool use: decode: %w", err)
	}
	use := ToolUse{ID: data.ToolUse.ID, SessionID: data.ToolUse.SessionID, RunID: data.ToolUse.RunID, ModelCallID: data.ToolUse.ModelCallID, AssistantMessageID: data.ToolUse.AssistantMessageID, PartID: data.ToolUse.PartID, Step: data.ToolUse.Step, Ordinal: data.ToolUse.Ordinal, CallID: data.ToolUse.CallID, Name: data.ToolUse.Name, Status: data.ToolUse.Status, InputJSON: data.ToolUse.InputJSON, Output: data.ToolUse.Output, StructuredJSON: data.ToolUse.StructuredJSON, MetadataJSON: data.ToolUse.MetadataJSON, ErrorType: data.ToolUse.ErrorType, ErrorMessage: data.ToolUse.ErrorMessage, ProposedAt: data.ToolUse.ProposedAt, AuthorizedAt: data.ToolUse.AuthorizedAt, StartedAt: data.ToolUse.StartedAt, CompletedAt: data.ToolUse.CompletedAt, CreatedAt: data.ToolUse.CreatedAt, UpdatedAt: data.ToolUse.UpdatedAt}
	if use.ID == "" || use.SessionID != event.Aggregate.ID || use.RunID != event.RunID || use.Status == "" || use.ProposedAt.IsZero() || use.CreatedAt.IsZero() || use.UpdatedAt.IsZero() {
		return ToolUse{}, fmt.Errorf("project session tool use: snapshot does not match aggregate event")
	}
	return use, nil
}

func marshalSessionRun(run SessionRun) (json.RawMessage, error) {
	runData, err := json.Marshal(run)
	if err != nil {
		return nil, fmt.Errorf("marshal run: %w", err)
	}
	var projection map[string]json.RawMessage
	if err := json.Unmarshal(runData, &projection); err != nil {
		return nil, fmt.Errorf("decode run: %w", err)
	}
	outputSchema, err := json.Marshal(string(run.OutputSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("marshal run output schema: %w", err)
	}
	projection["output_schema_json"] = outputSchema
	requestHash, err := json.Marshal(run.RequestHash)
	if err != nil {
		return nil, fmt.Errorf("marshal run request hash: %w", err)
	}
	projection["request_hash"] = requestHash
	runData, err = json.Marshal(projection)
	if err != nil {
		return nil, fmt.Errorf("marshal run projection: %w", err)
	}
	return runData, nil
}

func unmarshalSessionRun(data json.RawMessage) (SessionRun, error) {
	var run SessionRun
	if err := json.Unmarshal(data, &run); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode run: %w", err)
	}
	var encoded struct {
		OutputSchemaJSON string `json:"output_schema_json"`
		RequestHash      string `json:"request_hash"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode output schema: %w", err)
	}
	if encoded.OutputSchemaJSON != "" {
		run.OutputSchemaJSON = []byte(encoded.OutputSchemaJSON)
	}
	run.RequestHash = encoded.RequestHash
	return run, nil
}

// NewSessionCreatedEvent creates the initial fact for a session aggregate.
func NewSessionCreatedEvent(session Session) (AggregateEvent, error) {
	data, err := json.Marshal(sessionCreatedData{
		ID:          session.ID,
		Title:       session.Title,
		WorkDir:     session.WorkDir,
		WorkspaceID: session.WorkspaceID,
		ClientID:    session.ClientID,
		CreatedAt:   session.CreatedAt,
		UpdatedAt:   session.UpdatedAt,
	})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.created: %w", err)
	}
	eventTime, err := time.Parse(time.RFC3339Nano, session.CreatedAt)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("parse session created_at: %w", err)
	}
	return AggregateEvent{
		ID:            NewID(PrefixEvent),
		Aggregate:     AggregateRef{Type: AggregateSession, ID: session.ID},
		Type:          EventSessionCreated,
		SchemaVersion: 1,
		Time:          eventTime,
		Data:          data,
		ClientID:      session.ClientID,
	}, nil
}

// NewSessionRenamedEvent records a session title change.
func NewSessionRenamedEvent(sessionID, title, updatedAt string) (AggregateEvent, error) {
	data, err := json.Marshal(sessionRenamedData{Title: title, UpdatedAt: updatedAt})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.renamed: %w", err)
	}
	eventTime, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("parse session updated_at: %w", err)
	}
	return AggregateEvent{
		ID:            NewID(PrefixEvent),
		Aggregate:     AggregateRef{Type: AggregateSession, ID: sessionID},
		Type:          EventSessionRenamed,
		SchemaVersion: 1,
		Time:          eventTime,
		Data:          data,
	}, nil
}

// NewSessionMovedEvent records a session execution-location change.
func NewSessionMovedEvent(sessionID, workDir, workspaceID, updatedAt string) (AggregateEvent, error) {
	data, err := json.Marshal(sessionMovedData{WorkDir: workDir, WorkspaceID: workspaceID, UpdatedAt: updatedAt})
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal session.moved: %w", err)
	}
	eventTime, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("parse session updated_at: %w", err)
	}
	return AggregateEvent{
		ID:            NewID(PrefixEvent),
		Aggregate:     AggregateRef{Type: AggregateSession, ID: sessionID},
		Type:          EventSessionMoved,
		SchemaVersion: 1,
		Time:          eventTime,
		Data:          data,
	}, nil
}

// ProjectSession rebuilds a session projection from its ordered event stream.
func ProjectSession(events []AggregateEvent) (*Session, error) {
	var session *Session
	var version int64
	for _, event := range events {
		if event.Aggregate.Type != AggregateSession {
			return nil, fmt.Errorf("project session: unexpected aggregate type %q", event.Aggregate.Type)
		}
		if session != nil && event.Aggregate.ID != session.ID {
			return nil, fmt.Errorf("project session %s: event belongs to aggregate %s", session.ID, event.Aggregate.ID)
		}
		if event.Version != version+1 {
			return nil, fmt.Errorf("project session %s: expected event version %d, got %d", event.Aggregate.ID, version+1, event.Version)
		}
		if event.SchemaVersion != 1 {
			return nil, fmt.Errorf("project session %s: unsupported %s schema version %d", event.Aggregate.ID, event.Type, event.SchemaVersion)
		}
		var err error
		session, err = projectSessionEvent(session, event)
		if err != nil {
			return nil, err
		}
		version = event.Version
	}
	if session == nil {
		return nil, errors.New("project session: empty event stream")
	}
	return session, nil
}

// ProjectSessionRuns rebuilds session-run projections from a Session aggregate stream.
func ProjectSessionRuns(events []AggregateEvent) ([]SessionRun, error) {
	if _, err := ProjectSession(events); err != nil {
		return nil, err
	}
	runs := make(map[string]SessionRun)
	for _, event := range events {
		var run SessionRun
		var err error
		switch event.Type {
		case EventSessionRunAdmitted:
			run, err = ProjectSessionRunAdmission(event)
			if err != nil {
				return nil, err
			}
			if run.Status != SessionRunStatusQueued {
				return nil, fmt.Errorf("project session run %s: admission status %q", run.ID, run.Status)
			}
			if _, exists := runs[run.ID]; exists {
				return nil, fmt.Errorf("project session run %s: duplicate admission", run.ID)
			}
			runs[run.ID] = run
		case EventSessionRunStarted, EventSessionRunCompleted, EventSessionRunFailed, EventSessionRunAborted:
			run, err = ProjectSessionRunTransition(event)
			if err != nil {
				return nil, err
			}
			previous, exists := runs[run.ID]
			if !exists || !legalProjectedRunTransition(previous.Status, run.Status) {
				return nil, fmt.Errorf("project session run %s: illegal transition %q -> %q", run.ID, previous.Status, run.Status)
			}
			if run.Sequence != previous.Sequence || run.AdmittedVersion != previous.AdmittedVersion || run.Message != previous.Message {
				return nil, fmt.Errorf("project session run %s: immutable admission fields changed", run.ID)
			}
			runs[run.ID] = run
		}
	}
	out := make([]SessionRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, run)
	}
	slices.SortFunc(out, func(a, b SessionRun) int { return a.Sequence - b.Sequence })
	return out, nil
}

// ProjectSessionMessages rebuilds message projections from a Session aggregate stream.
func ProjectSessionMessages(events []AggregateEvent) ([]StoredMessage, error) {
	if _, err := ProjectSession(events); err != nil {
		return nil, err
	}
	messages := make(map[string]StoredMessage)
	indices := make(map[int]string)
	for _, event := range events {
		if event.Type != EventSessionMessageSaved {
			continue
		}
		message, err := ProjectSessionMessageSaved(event)
		if err != nil {
			return nil, err
		}
		previous, exists := messages[message.ID]
		if exists {
			if message.Revision <= previous.Revision || message.SessionID != previous.SessionID || message.RunID != previous.RunID || message.Idx != previous.Idx || message.Role != previous.Role {
				return nil, fmt.Errorf("project session message %s: invalid revision", message.ID)
			}
		} else if owner, exists := indices[message.Idx]; exists && owner != message.ID {
			return nil, fmt.Errorf("project session message %s: index %d belongs to %s", message.ID, message.Idx, owner)
		}
		messages[message.ID] = message
		indices[message.Idx] = message.ID
	}
	out := make([]StoredMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, message)
	}
	slices.SortFunc(out, func(a, b StoredMessage) int { return a.Idx - b.Idx })
	return out, nil
}

// ProjectSessionModelCalls rebuilds current model-call state from a Session aggregate stream.
func ProjectSessionModelCalls(events []AggregateEvent) ([]ModelCall, error) {
	if _, err := ProjectSession(events); err != nil {
		return nil, err
	}
	calls := make(map[string]ModelCall)
	type attemptKey struct {
		runID   string
		step    int
		attempt int
	}
	attempts := make(map[attemptKey]string)
	for _, event := range events {
		if event.Type != EventSessionModelCallSaved {
			continue
		}
		call, err := ProjectSessionModelCallSaved(event)
		if err != nil {
			return nil, err
		}
		if previous, exists := calls[call.ID]; exists {
			if call.SessionID != previous.SessionID || call.RunID != previous.RunID || call.Step != previous.Step || call.Attempt != previous.Attempt || !call.StartedAt.Equal(previous.StartedAt) || !call.CreatedAt.Equal(previous.CreatedAt) {
				return nil, fmt.Errorf("project session model call %s: immutable identity changed", call.ID)
			}
		} else if call.RunID != "" {
			key := attemptKey{runID: call.RunID, step: call.Step, attempt: call.Attempt}
			if owner, exists := attempts[key]; exists && owner != call.ID {
				return nil, fmt.Errorf("project session model call %s: attempt belongs to %s", call.ID, owner)
			}
			attempts[key] = call.ID
		}
		calls[call.ID] = call
	}
	out := make([]ModelCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, call)
	}
	slices.SortFunc(out, func(a, b ModelCall) int {
		if a.StartedAt.Equal(b.StartedAt) {
			return strings.Compare(a.ID, b.ID)
		}
		if a.StartedAt.Before(b.StartedAt) {
			return -1
		}
		return 1
	})
	return out, nil
}

// ProjectSessionToolUses rebuilds current tool-use state from a Session aggregate stream.
func ProjectSessionToolUses(events []AggregateEvent) ([]ToolUse, error) {
	if _, err := ProjectSession(events); err != nil {
		return nil, err
	}
	uses := make(map[string]ToolUse)
	identities := make(map[string]string)
	for _, event := range events {
		if event.Type != EventSessionToolUseSaved {
			continue
		}
		use, err := ProjectSessionToolUseSaved(event)
		if err != nil {
			return nil, err
		}
		if previous, exists := uses[use.ID]; exists {
			if !sameToolUseIdentity(previous, use) || !legalProjectedToolUseTransition(previous.Status, use.Status) {
				return nil, fmt.Errorf("project session tool use %s: invalid lifecycle snapshot", use.ID)
			}
		} else if use.Status != ToolUseStatusProposed {
			return nil, fmt.Errorf("project session tool use %s: lifecycle starts at %q", use.ID, use.Status)
		} else if use.RunID != "" {
			key := fmt.Sprintf("%s:%d:%d", use.RunID, use.Step, use.Ordinal)
			if owner, exists := identities[key]; exists && owner != use.ID {
				return nil, fmt.Errorf("project session tool use %s: identity belongs to %s", use.ID, owner)
			}
			identities[key] = use.ID
		}
		uses[use.ID] = use
	}
	out := make([]ToolUse, 0, len(uses))
	for _, use := range uses {
		out = append(out, use)
	}
	slices.SortFunc(out, func(a, b ToolUse) int {
		if a.ProposedAt.Equal(b.ProposedAt) {
			if a.Step == b.Step {
				if a.Ordinal == b.Ordinal {
					return strings.Compare(a.ID, b.ID)
				}
				return a.Ordinal - b.Ordinal
			}
			return a.Step - b.Step
		}
		if a.ProposedAt.Before(b.ProposedAt) {
			return -1
		}
		return 1
	})
	return out, nil
}

func legalProjectedToolUseTransition(from, to string) bool {
	return legalToolUseTransition(from, to)
}

func legalProjectedRunTransition(from, to string) bool {
	return (from == SessionRunStatusQueued && (to == SessionRunStatusRunning || to == SessionRunStatusAborted)) ||
		(from == SessionRunStatusRunning && isSessionRunTerminal(to))
}

func projectSessionEvent(session *Session, event AggregateEvent) (*Session, error) {
	switch event.Type {
	case EventSessionCreated:
		if session != nil {
			return nil, fmt.Errorf("project session %s: duplicate creation event", event.Aggregate.ID)
		}
		var data sessionCreatedData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, fmt.Errorf("project session %s: decode session.created: %w", event.Aggregate.ID, err)
		}
		if data.ID != event.Aggregate.ID {
			return nil, fmt.Errorf("project session %s: payload id %q does not match aggregate", event.Aggregate.ID, data.ID)
		}
		return &Session{
			ID:               data.ID,
			Title:            data.Title,
			WorkDir:          data.WorkDir,
			WorkspaceID:      data.WorkspaceID,
			ClientID:         data.ClientID,
			CreatedAt:        data.CreatedAt,
			UpdatedAt:        data.UpdatedAt,
			AggregateVersion: event.Version,
		}, nil
	case EventSessionRenamed:
		if session == nil {
			return nil, fmt.Errorf("project session %s: rename before creation", event.Aggregate.ID)
		}
		var data sessionRenamedData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, fmt.Errorf("project session %s: decode session.renamed: %w", event.Aggregate.ID, err)
		}
		projected := *session
		projected.Title = data.Title
		projected.UpdatedAt = data.UpdatedAt
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionMoved:
		if session == nil {
			return nil, fmt.Errorf("project session %s: move before creation", event.Aggregate.ID)
		}
		var data sessionMovedData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, fmt.Errorf("project session %s: decode session.moved: %w", event.Aggregate.ID, err)
		}
		projected := *session
		projected.WorkDir = data.WorkDir
		projected.WorkspaceID = data.WorkspaceID
		projected.UpdatedAt = data.UpdatedAt
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionRunAdmitted:
		if session == nil {
			return nil, fmt.Errorf("project session %s: run admission before creation", event.Aggregate.ID)
		}
		if _, err := ProjectSessionRunAdmission(event); err != nil {
			return nil, err
		}
		projected := *session
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionRunStarted, EventSessionRunCompleted, EventSessionRunFailed, EventSessionRunAborted:
		if session == nil {
			return nil, fmt.Errorf("project session %s: run transition before creation", event.Aggregate.ID)
		}
		if _, err := ProjectSessionRunTransition(event); err != nil {
			return nil, err
		}
		projected := *session
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionMessageSaved:
		if session == nil {
			return nil, fmt.Errorf("project session %s: message before creation", event.Aggregate.ID)
		}
		if _, err := ProjectSessionMessageSaved(event); err != nil {
			return nil, err
		}
		projected := *session
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionModelCallSaved:
		if session == nil {
			return nil, fmt.Errorf("project session %s: model call before creation", event.Aggregate.ID)
		}
		if _, err := ProjectSessionModelCallSaved(event); err != nil {
			return nil, err
		}
		projected := *session
		projected.AggregateVersion = event.Version
		return &projected, nil
	case EventSessionToolUseSaved:
		if session == nil {
			return nil, fmt.Errorf("project session %s: tool use before creation", event.Aggregate.ID)
		}
		if _, err := ProjectSessionToolUseSaved(event); err != nil {
			return nil, err
		}
		projected := *session
		projected.AggregateVersion = event.Version
		return &projected, nil
	default:
		return nil, fmt.Errorf("project session %s: unsupported event type %q", event.Aggregate.ID, event.Type)
	}
}
