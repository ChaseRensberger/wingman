package store

import (
	"context"
	"errors"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrClientNameExists = errors.New("client name already exists")
var ErrClientIDExists = errors.New("client ID already exists")
var ErrWorkspaceNameExists = errors.New("workspace name already exists")
var ErrSessionRunAdmissionConflict = errors.New("session run admission conflict")
var ErrSessionRunNotFound = errors.New("session run not found")
var ErrSessionRunTransitionConflict = errors.New("session run transition conflict")
var ErrModelCallAttemptConflict = errors.New("model call attempt conflict")
var ErrToolUseIdentityConflict = errors.New("tool use identity conflict")
var ErrToolUseInvalidTransition = errors.New("tool use invalid transition")
var ErrMessageRevisionStale = errors.New("message revision stale")
var ErrMessageRevisionConflict = errors.New("message revision conflict")
var ErrPermissionRequestNotFound = errors.New("permission request not found")
var ErrPermissionRequestTransitionConflict = errors.New("permission request transition conflict")

// PermissionRequestNotFound identifies a request absent from a session.
type PermissionRequestNotFound struct{ SessionID, RequestID string }

func (e *PermissionRequestNotFound) Error() string {
	return "session " + e.SessionID + ": " + ErrPermissionRequestNotFound.Error() + ": " + e.RequestID
}
func (e *PermissionRequestNotFound) Unwrap() error { return ErrPermissionRequestNotFound }

// PermissionRequestTransitionConflict reports an invalid or competing resolution.
type PermissionRequestTransitionConflict struct{ SessionID, RequestID string }

func (e *PermissionRequestTransitionConflict) Error() string {
	return "session " + e.SessionID + ": " + ErrPermissionRequestTransitionConflict.Error() + ": " + e.RequestID
}
func (e *PermissionRequestTransitionConflict) Unwrap() error {
	return ErrPermissionRequestTransitionConflict
}

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
	GetSessionRun(ctx context.Context, sessionID, runID string) (*SessionRun, error)
	ListSessionRuns(ctx context.Context, sessionID string) ([]SessionRun, error)
	ClaimNextSessionRun(ctx context.Context, sessionID string) (SessionRunTransition, error)
	SettleSessionRun(ctx context.Context, settlement SessionRunSettlement) (SessionRunTransition, error)
	ListRunningSessionRuns(ctx context.Context) ([]SessionRun, error)
	ListQueuedSessionRunSessions(ctx context.Context) ([]string, error)
	CountQueuedSessionRuns(ctx context.Context) (int, error)
	CreatePermissionRequest(ctx context.Context, request PermissionRequest) (PermissionRequestTransition, error)
	GetPermissionRequest(ctx context.Context, sessionID, requestID string) (*PermissionRequest, error)
	ListPermissionRequests(ctx context.Context, sessionID string) ([]PermissionRequest, error)
	ResolvePermissionRequest(ctx context.Context, resolution PermissionRequestResolution) (PermissionRequestTransition, error)
	ListPermissionGrants(ctx context.Context, sessionID string) ([]PermissionGrant, error)
	InterruptPendingPermissionRequests(ctx context.Context) ([]PermissionRequestTransition, error)

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
	InterruptActiveModelCalls(ctx context.Context, runID, errorType, errorMessage string) error
	SaveToolUse(ctx context.Context, use ToolUse) error
	ListToolUses(ctx context.Context, sessionID string) ([]ToolUse, error)
	InterruptActiveToolUses(ctx context.Context) error
	// AppendSessionEvent stores one durable session event and assigns its
	// session-scoped sequence.
	AppendSessionEvent(ctx context.Context, event SessionEvent) (SessionEvent, error)
	// ListSessionEvents returns durable session events with Seq > after.
	ListSessionEvents(ctx context.Context, sessionID string, after int64, limit int) ([]SessionEvent, error)
	// SessionEventWatermark returns the highest durable session-event sequence,
	// or zero when the existing session has no events.
	SessionEventWatermark(ctx context.Context, sessionID string) (int64, error)

	// CreateClient registers a Wingman API consumer identity.
	CreateClient(name string) (*Client, error)
	CreateClientWithID(id, name string) (*Client, error)
	EnsureDefaultClient() (*Client, error)
	GetClient(id string) (*Client, error)
	ListClients() ([]*Client, error)
	CreateAuthSession(session *AuthSession) error
	AuthenticateAuthSession(tokenHash string) (*AuthSession, error)
	ListAuthSessions(clientID string) ([]*AuthSession, error)
	RevokeAuthSession(id string) error

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
