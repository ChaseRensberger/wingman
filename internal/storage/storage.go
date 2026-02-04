package storage

type Store interface {
	CreateAgent(agent *Agent) error
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	UpdateAgent(agent *Agent) error
	DeleteAgent(id string) error

	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(id string) error

	CreateFleet(fleet *Fleet) error
	GetFleet(id string) (*Fleet, error)
	ListFleets() ([]*Fleet, error)
	UpdateFleet(fleet *Fleet) error
	DeleteFleet(id string) error

	CreateFormation(formation *Formation) error
	GetFormation(id string) (*Formation, error)
	ListFormations() ([]*Formation, error)
	UpdateFormation(formation *Formation) error
	DeleteFormation(id string) error

	GetAuth() (*Auth, error)
	SetAuth(auth *Auth) error

	Close() error
}
