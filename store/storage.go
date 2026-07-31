package store

import (
	"context"
	"errors"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrClientNameExists = errors.New("client name already exists")
var ErrWorkspaceNameExists = errors.New("workspace name already exists")
var ErrSessionRunAdmissionConflict = errors.New("session run admission conflict")
var ErrModelCallAttemptConflict = errors.New("model call attempt conflict")
var ErrMessageRevisionStale = errors.New("message revision stale")
var ErrMessageRevisionConflict = errors.New("message revision conflict")

// SessionRunAdmissionConflict reports conflicting reuse of a request identity.
type SessionRunAdmissionConflict struct {
	SessionID string
	RequestID string
}

func (e *SessionRunAdmissionConflict) Error() string {
	return "session " + e.SessionID + ": " + ErrSessionRunAdmissionConflict.Error() + ": request_id " + e.RequestID
}

func (e *SessionRunAdmissionConflict) Unwrap() error { return ErrSessionRunAdmissionConflict }

type Store interface {
	CreateAgent(agent *Agent) error
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	UpdateAgent(agent *Agent) error
	DeleteAgent(id string) error

	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	ListSessionsByClient(clientID string) ([]*Session, error)
	RenameSession(ctx context.Context, id, title string, expectedVersion int64) (*Session, error)
	MoveSession(ctx context.Context, id, workDir, workspaceID string, expectedVersion int64) (*Session, error)
	PurgeSession(ctx context.Context, id string, expectedVersion int64) error
	AdmitSessionRun(ctx context.Context, run SessionRun) (SessionRunAdmission, error)
	ClaimNextSessionRun(ctx context.Context, sessionID string) (*SessionRun, error)
	CompleteSessionRun(ctx context.Context, id, status, errorMessage string) error
	ListQueuedSessionRunSessions(ctx context.Context) ([]string, error)
	AbortRunningSessionRuns(ctx context.Context) error

	// SaveMessage atomically stores a complete authoritative message revision.
	SaveMessage(ctx context.Context, msg StoredMessage) error
	// ListMessages returns all messages for the session ordered by Idx
	// ASC, with each message's Parts populated and ordered by Sequence
	// ASC. Returns ErrSessionNotFound if the session does not exist.
	// Returns an empty slice (not nil) when the session has no messages.
	ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error)
	// UpsertModelCall inserts or updates one upstream model-call record keyed by ID.
	UpsertModelCall(ctx context.Context, call ModelCall) error
	// LatestModelCall returns the latest call with context usage for a session.
	LatestModelCall(ctx context.Context, sessionID string) (*ModelCall, error)
	// ListModelCalls returns all model calls for the session in chronological order.
	ListModelCalls(ctx context.Context, sessionID string) ([]ModelCall, error)
	// AppendSessionEvent stores one durable session event and assigns its
	// session-scoped sequence.
	AppendSessionEvent(ctx context.Context, event SessionEvent) (SessionEvent, error)
	// ListSessionEvents returns durable session events with Seq > after.
	ListSessionEvents(ctx context.Context, sessionID string, after int64, limit int) ([]SessionEvent, error)

	// CreateClient registers a Wingman API consumer identity.
	CreateClient(name string) (*Client, error)
	EnsureDefaultClient() (*Client, error)
	GetClient(id string) (*Client, error)
	ListClients() ([]*Client, error)

	CreateWorkspace(workspace *Workspace) error
	GetWorkspace(id string) (*Workspace, error)
	ListWorkspaces() ([]*Workspace, error)
	ListWorkspacesByClient(clientID string) ([]*Workspace, error)
	UpdateWorkspace(workspace *Workspace) error
	DeleteWorkspace(id string) error
	ListSessionsByWorkspace(workspaceID string) ([]*Session, error)

	GetAuth() (*Auth, error)
	SetAuth(auth *Auth) error

	Close() error
}

// AggregateEventReader exposes immutable domain history to replay and
// diagnostic consumers without widening the runtime Store contract.
type AggregateEventReader interface {
	ListAggregateEvents(ctx context.Context, aggregate AggregateRef, afterVersion int64, limit int) ([]AggregateEvent, error)
}
