// Package memory provides an in-memory implementation of store.Store
// suitable for tests and ephemeral runs. It conforms to the same
// behavioral contract as store/sqlite.go (verified by store/storetest).
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/chaserensberger/wingman/store"
)

// Store is an in-memory implementation of store.Store protected by a
// single sync.RWMutex.
type Store struct {
	mu       sync.RWMutex
	agents   map[string]*store.Agent
	sessions map[string]*store.Session
	clients  map[string]*store.Client
	messages map[string]*store.StoredMessage
	parts    map[string]*store.StoredPart
	auth     *store.Auth
}

// NewStore returns a fresh empty in-memory store.
func NewStore() *Store {
	return &Store{
		agents:   make(map[string]*store.Agent),
		sessions: make(map[string]*store.Session),
		clients:  make(map[string]*store.Client),
		messages: make(map[string]*store.StoredMessage),
		parts:    make(map[string]*store.StoredPart),
	}
}

// Close is a no-op for the in-memory store.
func (s *Store) Close() error { return nil }

// ---- defensive copying helpers ------------------------------------------

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	json.Unmarshal(b, &out) //nolint:errcheck
	return out
}

func copyAgent(a *store.Agent) *store.Agent {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Tools = make([]string, len(a.Tools))
	copy(cp.Tools, a.Tools)
	cp.Options = deepCopyMap(a.Options)
	cp.OutputSchema = deepCopyMap(a.OutputSchema)
	return &cp
}

func copySession(sess *store.Session) *store.Session {
	if sess == nil {
		return nil
	}
	cp := *sess
	return &cp
}

func copyClient(c *store.Client) *store.Client {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

func copyMessage(m *store.StoredMessage) store.StoredMessage {
	cp := *m
	if m.MetadataJSON != nil {
		cp.MetadataJSON = make([]byte, len(m.MetadataJSON))
		copy(cp.MetadataJSON, m.MetadataJSON)
	}
	cp.Parts = make([]store.StoredPart, len(m.Parts))
	for i, p := range m.Parts {
		cp.Parts[i] = copyPart(&p)
	}
	return cp
}

func copyPart(p *store.StoredPart) store.StoredPart {
	cp := *p
	if p.PayloadJSON != nil {
		cp.PayloadJSON = make([]byte, len(p.PayloadJSON))
		copy(cp.PayloadJSON, p.PayloadJSON)
	}
	return cp
}

func copyAuth(a *store.Auth) *store.Auth {
	if a == nil {
		return &store.Auth{Providers: make(map[string]store.AuthCredential)}
	}
	cp := &store.Auth{
		UpdatedAt: a.UpdatedAt,
		Providers: make(map[string]store.AuthCredential, len(a.Providers)),
	}
	for k, v := range a.Providers {
		cp.Providers[k] = v
	}
	return cp
}

// ---- agents --------------------------------------------------------------

func (s *Store) CreateAgent(agent *store.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if agent.ID == "" {
		agent.ID = store.NewID(store.PrefixAgent)
	}
	now := store.Now()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	s.agents[agent.ID] = copyAgent(agent)
	return nil
}

func (s *Store) GetAgent(id string) (*store.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return copyAgent(a), nil
}

func (s *Store) ListAgents() ([]*store.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, copyAgent(a))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) UpdateAgent(agent *store.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.agents[agent.ID]
	if !ok {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}

	agent.UpdatedAt = store.Now()
	agent.CreatedAt = existing.CreatedAt
	s.agents[agent.ID] = copyAgent(agent)
	return nil
}

func (s *Store) DeleteAgent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[id]; !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	delete(s.agents, id)
	return nil
}

// ---- clients -------------------------------------------------------------

func (s *Store) CreateClient(name string) (*store.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &store.Client{
		ID:        store.NewID(store.PrefixClient),
		Name:      name,
		CreatedAt: store.Now(),
	}
	s.clients[client.ID] = copyClient(client)
	return client, nil
}

func (s *Store) GetClient(id string) (*store.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.clients[id]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", id)
	}
	return copyClient(c), nil
}

func (s *Store) ListClients() ([]*store.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Client, 0, len(s.clients))
	for _, c := range s.clients {
		out = append(out, copyClient(c))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

// ---- sessions ------------------------------------------------------------

func (s *Store) CreateSession(session *store.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session.ID == "" {
		session.ID = store.NewID(store.PrefixSession)
	}
	now := store.Now()
	session.CreatedAt = now
	session.UpdatedAt = now

	if session.ClientID != "" {
		if _, ok := s.clients[session.ClientID]; !ok {
			return fmt.Errorf("client not found: %s", session.ClientID)
		}
	}

	s.sessions[session.ID] = copySession(session)
	return nil
}

func (s *Store) GetSession(id string) (*store.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return copySession(sess), nil
}

func (s *Store) ListSessions() ([]*store.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, copySession(sess))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) ListSessionsByClient(clientID string) ([]*store.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Session, 0)
	for _, sess := range s.sessions {
		if sess.ClientID == clientID {
			out = append(out, copySession(sess))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) UpdateSession(session *store.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.sessions[session.ID]
	if !ok {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	session.UpdatedAt = store.Now()
	session.ClientID = existing.ClientID
	session.CreatedAt = existing.CreatedAt
	s.sessions[session.ID] = copySession(session)
	return nil
}

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	msgIDs := make(map[string]struct{})
	for msgID, msg := range s.messages {
		if msg.SessionID == id {
			msgIDs[msgID] = struct{}{}
		}
	}
	for msgID := range msgIDs {
		delete(s.messages, msgID)
	}
	for partID, part := range s.parts {
		if _, ok := msgIDs[part.MessageID]; ok {
			delete(s.parts, partID)
		}
	}

	delete(s.sessions, id)
	return nil
}

// ---- messages and parts --------------------------------------------------

func (s *Store) UpsertMessage(ctx context.Context, msg store.StoredMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.messages[msg.ID]; ok {
		msg.CreatedAt = existing.CreatedAt
		msg.Idx = existing.Idx
	}
	msg.Parts = nil
	if msg.MetadataJSON != nil {
		b := make([]byte, len(msg.MetadataJSON))
		copy(b, msg.MetadataJSON)
		msg.MetadataJSON = b
	}
	s.messages[msg.ID] = &msg
	return nil
}

func (s *Store) UpsertPart(ctx context.Context, part store.StoredPart) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.parts[part.ID]; ok {
		part.CreatedAt = existing.CreatedAt
		part.Sequence = existing.Sequence
	}
	if part.PayloadJSON != nil {
		b := make([]byte, len(part.PayloadJSON))
		copy(b, part.PayloadJSON)
		part.PayloadJSON = b
	}
	s.parts[part.ID] = &part
	return nil
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]store.StoredMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}

	var msgs []store.StoredMessage
	for _, msg := range s.messages {
		if msg.SessionID == sessionID {
			msgs = append(msgs, copyMessage(msg))
		}
	}
	if len(msgs) == 0 {
		return []store.StoredMessage{}, nil
	}

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Idx < msgs[j].Idx
	})

	msgMap := make(map[string]*store.StoredMessage, len(msgs))
	for i := range msgs {
		msgMap[msgs[i].ID] = &msgs[i]
	}

	for _, part := range s.parts {
		if m, ok := msgMap[part.MessageID]; ok {
			m.Parts = append(m.Parts, copyPart(part))
		}
	}

	for i := range msgs {
		sort.Slice(msgs[i].Parts, func(a, b int) bool {
			return msgs[i].Parts[a].Sequence < msgs[i].Parts[b].Sequence
		})
	}

	return msgs, nil
}

// ---- auth ----------------------------------------------------------------

func (s *Store) GetAuth() (*store.Auth, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return copyAuth(s.auth), nil
}

func (s *Store) SetAuth(auth *store.Auth) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	auth.UpdatedAt = store.Now()
	s.auth = copyAuth(auth)
	return nil
}
