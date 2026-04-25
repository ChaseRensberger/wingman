package storage

import "github.com/chaserensberger/wingman/wingmodels"

type Store interface {
	CreateAgent(agent *Agent) error
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	UpdateAgent(agent *Agent) error
	DeleteAgent(id string) error

	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	// UpdateSession persists metadata-only fields (work_dir,
	// updated_at). It does NOT touch the message history; use
	// AppendMessage for incremental appends or ReplaceMessages for
	// full rewrites.
	UpdateSession(session *Session) error
	// AppendMessage appends a single message (and its parts) to the
	// session's history at the next index. Use this from message-sink
	// callbacks during a Run for incremental persistence.
	AppendMessage(sessionID string, msg wingmodels.Message) error
	// ReplaceMessages atomically clears the session's existing
	// history and writes msgs in order. Reserved for power users
	// (rehydration tools, history editors); routine traffic should
	// use AppendMessage.
	ReplaceMessages(sessionID string, msgs []wingmodels.Message) error
	DeleteSession(id string) error

	GetAuth() (*Auth, error)
	SetAuth(auth *Auth) error

	Close() error
}
