package store

import (
	"context"
	"errors"
)

var ErrSessionNotFound = errors.New("session not found")

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
	// UpdateSession persists mutable session metadata.
	UpdateSession(session *Session) error
	DeleteSession(id string) error

	// UpsertMessage inserts or updates a message row keyed by ID.
	// It does not touch parts.
	UpsertMessage(ctx context.Context, msg StoredMessage) error
	// UpsertPart inserts or updates a part row keyed by ID.
	UpsertPart(ctx context.Context, part StoredPart) error
	// ListMessages returns all messages for the session ordered by Idx
	// ASC, with each message's Parts populated and ordered by Sequence
	// ASC. Returns ErrSessionNotFound if the session does not exist.
	// Returns an empty slice (not nil) when the session has no messages.
	ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error)

	// CreateClient registers a Wingman API consumer identity.
	CreateClient(name string) (*Client, error)
	GetClient(id string) (*Client, error)
	ListClients() ([]*Client, error)

	GetAuth() (*Auth, error)
	SetAuth(auth *Auth) error

	Close() error
}
