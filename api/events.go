package api

import (
	"encoding/json"
	"fmt"

	"github.com/chaserensberger/wingman/models"
)

// SessionEventType identifies one persistent-session event payload.
type SessionEventType string

const (
	SessionEventRunQueued                 SessionEventType = "session.run.queued"
	SessionEventRunStarted                SessionEventType = "session.run.started"
	SessionEventRunCompleted              SessionEventType = "session.run.completed"
	SessionEventRunFailed                 SessionEventType = "session.run.failed"
	SessionEventRunAborted                SessionEventType = "session.run.aborted"
	SessionEventStepStarted               SessionEventType = "session.step.started"
	SessionEventStepCompleted             SessionEventType = "session.step.completed"
	SessionEventTextDelta                 SessionEventType = "session.text.delta"
	SessionEventTextCompleted             SessionEventType = "session.text.completed"
	SessionEventReasoningDelta            SessionEventType = "session.reasoning.delta"
	SessionEventReasoningCompleted        SessionEventType = "session.reasoning.completed"
	SessionEventMessageCreated            SessionEventType = "session.message.created"
	SessionEventToolCalled                SessionEventType = "session.tool.called"
	SessionEventToolUpdated               SessionEventType = "session.tool.updated"
	SessionEventToolInputDelta            SessionEventType = "session.tool.input.delta"
	SessionEventToolProgress              SessionEventType = "session.tool.progress"
	SessionEventToolCompleted             SessionEventType = "session.tool.completed"
	SessionEventToolFailed                SessionEventType = "session.tool.failed"
	SessionEventPermissionRequested       SessionEventType = "session.permission.requested"
	SessionEventPermissionResolved        SessionEventType = "session.permission.resolved"
	SessionEventStructuredOutputCompleted SessionEventType = "session.structured_output.completed"
	SessionEventEventsSynchronized        SessionEventType = "session.events.synchronized"
	SessionEventEventsResyncRequired      SessionEventType = "session.events.resync_required"
)

// SessionEventCursor is the exclusive durable resume position for a session.
type SessionEventCursor struct {
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

// SessionEvent is the canonical persistent-session SSE and history envelope.
type SessionEvent struct {
	ID     string              `json:"id"`
	Type   SessionEventType    `json:"type"`
	Time   string              `json:"time,omitempty"`
	Cursor *SessionEventCursor `json:"cursor,omitempty"`
	Data   SessionEventData    `json:"data"`
}

// SessionEventPage is one page of durable session events.
type SessionEventPage struct {
	Data    []SessionEvent `json:"data"`
	HasMore bool           `json:"has_more"`
}

// SessionEventData is the closed union of known session event payloads.
type SessionEventData interface {
	isSessionEventData()
}

type sessionEventData struct{}

func (sessionEventData) isSessionEventData() {}

// RunEventData describes admission and run lifecycle events.
type RunEventData struct {
	sessionEventData
	RunID        string        `json:"run_id"`
	Status       string        `json:"status,omitempty"`
	Message      string        `json:"message,omitempty"`
	ErrorType    string        `json:"error_type,omitempty"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Usage        *models.Usage `json:"usage,omitempty"`
	Steps        int           `json:"steps,omitempty"`
	StartedAt    string        `json:"started_at,omitempty"`
	CompletedAt  string        `json:"completed_at,omitempty"`
	UpdatedAt    string        `json:"updated_at,omitempty"`
}

// StepEventData describes one model/tool loop iteration.
type StepEventData struct {
	sessionEventData
	RunID string        `json:"run_id"`
	Step  int           `json:"step"`
	Usage *models.Usage `json:"usage,omitempty"`
}

// ContentDeltaEventData carries volatile text, reasoning, or tool-input data.
type ContentDeltaEventData struct {
	sessionEventData
	RunID     string `json:"run_id"`
	Step      int    `json:"step,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	PartID    string `json:"part_id,omitempty"`
	Revision  int64  `json:"revision,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Delta     string `json:"delta"`
}

// ContentCompletedEventData carries one durable completed text or reasoning part.
type ContentCompletedEventData struct {
	sessionEventData
	RunID     string `json:"run_id"`
	MessageID string `json:"message_id,omitempty"`
	PartID    string `json:"part_id,omitempty"`
	Revision  int64  `json:"revision,omitempty"`
	Text      string `json:"text"`
}

// MessageCreatedEventData carries one durable message snapshot.
type MessageCreatedEventData struct {
	sessionEventData
	RunID   string         `json:"run_id"`
	Message models.Message `json:"message"`
}

// ToolEventData describes a tool call, lifecycle update, result, or progress.
type ToolEventData struct {
	sessionEventData
	RunID        string         `json:"run_id"`
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	CallID       string         `json:"call_id,omitempty"`
	Tool         string         `json:"tool,omitempty"`
	Status       string         `json:"status,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
	Output       string         `json:"output,omitempty"`
	OutputDelta  string         `json:"output_delta,omitempty"`
	Structured   any            `json:"structured,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Error        string         `json:"error,omitempty"`
	Step         int            `json:"step,omitempty"`
	Ordinal      int            `json:"ordinal,omitempty"`
	MessageID    string         `json:"message_id,omitempty"`
	PartID       string         `json:"part_id,omitempty"`
	Revision     int64          `json:"revision,omitempty"`
	ModelCallID  string         `json:"model_call_id,omitempty"`
	ProposedAt   string         `json:"proposed_at,omitempty"`
	AuthorizedAt string         `json:"authorized_at,omitempty"`
	StartedAt    string         `json:"started_at,omitempty"`
	CompletedAt  string         `json:"completed_at,omitempty"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
}

// PermissionEventData carries the current permission request projection.
type PermissionEventData struct {
	sessionEventData
	PermissionRequest
}

// StructuredOutputEventData carries validated structured output.
type StructuredOutputEventData struct {
	sessionEventData
	RunID   string         `json:"run_id"`
	Schema  string         `json:"schema,omitempty"`
	RawJSON string         `json:"raw_json"`
	Parsed  map[string]any `json:"parsed"`
}

// EventsSynchronizedEventData marks the durable/live stream boundary.
type EventsSynchronizedEventData struct {
	sessionEventData
	Cursor    int64 `json:"cursor"`
	Watermark int64 `json:"watermark"`
}

// EventsResyncRequiredEventData tells a client to reload authoritative state.
type EventsResyncRequiredEventData struct {
	sessionEventData
	Cursor int64  `json:"cursor"`
	Reason string `json:"reason"`
}

// UnknownSessionEventData preserves an event type not known by this build.
type UnknownSessionEventData struct {
	sessionEventData
	Raw json.RawMessage
}

// MarshalJSON preserves an unknown payload without wrapping it.
func (d UnknownSessionEventData) MarshalJSON() ([]byte, error) {
	if len(d.Raw) == 0 {
		return []byte(`{}`), nil
	}
	return d.Raw, nil
}

// DecodeSessionEventData decodes a payload according to its event discriminator.
// Unknown discriminators remain opaque for forward-compatible replay.
func DecodeSessionEventData(eventType SessionEventType, raw json.RawMessage) (SessionEventData, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var data SessionEventData
	switch eventType {
	case SessionEventRunQueued, SessionEventRunStarted, SessionEventRunCompleted, SessionEventRunFailed, SessionEventRunAborted:
		data = &RunEventData{}
	case SessionEventStepStarted, SessionEventStepCompleted:
		data = &StepEventData{}
	case SessionEventTextDelta, SessionEventReasoningDelta, SessionEventToolInputDelta:
		data = &ContentDeltaEventData{}
	case SessionEventTextCompleted, SessionEventReasoningCompleted:
		data = &ContentCompletedEventData{}
	case SessionEventMessageCreated:
		data = &MessageCreatedEventData{}
	case SessionEventToolCalled, SessionEventToolUpdated, SessionEventToolProgress, SessionEventToolCompleted, SessionEventToolFailed:
		data = &ToolEventData{}
	case SessionEventPermissionRequested, SessionEventPermissionResolved:
		data = &PermissionEventData{}
	case SessionEventStructuredOutputCompleted:
		data = &StructuredOutputEventData{}
	case SessionEventEventsSynchronized:
		data = &EventsSynchronizedEventData{}
	case SessionEventEventsResyncRequired:
		data = &EventsResyncRequiredEventData{}
	default:
		return UnknownSessionEventData{Raw: append(json.RawMessage(nil), raw...)}, nil
	}
	if err := json.Unmarshal(raw, data); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", eventType, err)
	}
	return data, nil
}
