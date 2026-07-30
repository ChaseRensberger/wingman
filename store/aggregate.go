package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AggregateType identifies a durable consistency and ordering boundary.
type AggregateType string

const (
	AggregateSession AggregateType = "session"
)

const (
	EventSessionCreated = "session.created"
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

// ProjectSession rebuilds a session projection from its ordered event stream.
func ProjectSession(events []AggregateEvent) (*Session, error) {
	var session *Session
	var version int64
	for _, event := range events {
		if event.Aggregate.Type != AggregateSession {
			return nil, fmt.Errorf("project session: unexpected aggregate type %q", event.Aggregate.Type)
		}
		if event.Version != version+1 {
			return nil, fmt.Errorf("project session %s: expected event version %d, got %d", event.Aggregate.ID, version+1, event.Version)
		}
		if event.SchemaVersion != 1 {
			return nil, fmt.Errorf("project session %s: unsupported %s schema version %d", event.Aggregate.ID, event.Type, event.SchemaVersion)
		}
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
			session = &Session{
				ID:               data.ID,
				Title:            data.Title,
				WorkDir:          data.WorkDir,
				WorkspaceID:      data.WorkspaceID,
				ClientID:         data.ClientID,
				CreatedAt:        data.CreatedAt,
				UpdatedAt:        data.UpdatedAt,
				AggregateVersion: event.Version,
			}
		default:
			return nil, fmt.Errorf("project session %s: unsupported event type %q", event.Aggregate.ID, event.Type)
		}
		version = event.Version
	}
	if session == nil {
		return nil, errors.New("project session: empty event stream")
	}
	return session, nil
}
