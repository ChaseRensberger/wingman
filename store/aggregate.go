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
	EventSessionCreated     = "session.created"
	EventSessionRenamed     = "session.renamed"
	EventSessionMoved       = "session.moved"
	EventSessionRunAdmitted = "session.run.admitted"
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

// NewSessionRunAdmittedEvent records an immutable admitted run projection.
func NewSessionRunAdmittedEvent(run SessionRun) (AggregateEvent, error) {
	runData, err := json.Marshal(run)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal admitted run: %w", err)
	}
	var projection map[string]json.RawMessage
	if err := json.Unmarshal(runData, &projection); err != nil {
		return AggregateEvent{}, fmt.Errorf("decode admitted run: %w", err)
	}
	outputSchema, err := json.Marshal(string(run.OutputSchemaJSON))
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("marshal admitted run output schema: %w", err)
	}
	projection["output_schema_json"] = outputSchema
	runData, err = json.Marshal(projection)
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
	var run SessionRun
	if err := json.Unmarshal(data.Run, &run); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode run: %w", err)
	}
	var encoded struct {
		OutputSchemaJSON string `json:"output_schema_json"`
	}
	if err := json.Unmarshal(data.Run, &encoded); err != nil {
		return SessionRun{}, fmt.Errorf("project session run: decode output schema: %w", err)
	}
	if run.SessionID != event.Aggregate.ID {
		return SessionRun{}, fmt.Errorf("project session run: payload session %q does not match aggregate", run.SessionID)
	}
	if run.AdmittedVersion != event.Version {
		return SessionRun{}, fmt.Errorf("project session run %s: payload admitted version %d does not match event version %d", run.ID, run.AdmittedVersion, event.Version)
	}
	run.OutputSchemaJSON = []byte(encoded.OutputSchemaJSON)
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
	default:
		return nil, fmt.Errorf("project session %s: unsupported event type %q", event.Aggregate.ID, event.Type)
	}
}
