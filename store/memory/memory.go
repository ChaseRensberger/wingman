// Package memory provides an in-memory implementation of store.Store
// suitable for ephemeral runs. It conforms to the same behavioral contract
// as store/sqlite.go.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/store"
)

// Store is an in-memory implementation of store.Store protected by a
// single sync.RWMutex.
type Store struct {
	mu         sync.RWMutex
	agents     map[string]*store.Agent
	sessions   map[string]*store.Session
	clients    map[string]*store.Client
	workspaces map[string]*store.Workspace
	messages   map[string]*store.StoredMessage
	parts      map[string]*store.StoredPart
	modelCalls map[string]*store.ModelCall
	events     map[string]*store.SessionEvent
	aggregates map[store.AggregateRef][]store.AggregateEvent
	globalSeq  int64
	runs       map[string]*store.SessionRun
	auth       *store.Auth
}

// NewStore returns a fresh empty in-memory store.
func NewStore() *Store {
	return &Store{
		agents:     make(map[string]*store.Agent),
		sessions:   make(map[string]*store.Session),
		clients:    make(map[string]*store.Client),
		workspaces: make(map[string]*store.Workspace),
		messages:   make(map[string]*store.StoredMessage),
		parts:      make(map[string]*store.StoredPart),
		modelCalls: make(map[string]*store.ModelCall),
		events:     make(map[string]*store.SessionEvent),
		aggregates: make(map[store.AggregateRef][]store.AggregateEvent),
		runs:       make(map[string]*store.SessionRun),
	}
}

// Close is a no-op for the in-memory store.
func (s *Store) Close() error { return nil }

func (s *Store) AdmitSessionRun(ctx context.Context, run store.SessionRun) (store.SessionRunAdmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[run.SessionID]
	if !ok {
		return store.SessionRunAdmission{}, store.ErrSessionNotFound
	}
	run.WorkDir, run.WorkspaceID, run.ClientID = session.WorkDir, session.WorkspaceID, session.ClientID
	hash, err := store.SessionRunRequestHash(run)
	if err != nil {
		return store.SessionRunAdmission{}, err
	}
	if run.RequestID != "" {
		for _, existing := range s.runs {
			if existing.SessionID == run.SessionID && existing.RequestID == run.RequestID {
				if existing.RequestHash != hash {
					return store.SessionRunAdmission{}, &store.SessionRunAdmissionConflict{SessionID: run.SessionID, RequestID: run.RequestID}
				}
				return store.SessionRunAdmission{Run: copySessionRun(existing), SessionVersion: session.AggregateVersion}, nil
			}
		}
	}
	if run.ID == "" {
		run.ID = store.NewID(store.PrefixRun)
	}
	next := 1
	for _, existing := range s.runs {
		if existing.SessionID == run.SessionID && existing.Sequence >= next {
			next = existing.Sequence + 1
		}
	}
	now := time.Now().UTC()
	run.Sequence = next
	run.Status, run.RequestHash = store.SessionRunStatusQueued, hash
	run.AdmittedVersion = session.AggregateVersion + 1
	run.CreatedAt = now
	run.UpdatedAt = now
	event, err := store.NewSessionRunAdmittedEvent(run)
	if err != nil {
		return store.SessionRunAdmission{}, err
	}
	event.Version = run.AdmittedVersion
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	queuedData, err := json.Marshal(struct {
		RunID   string `json:"run_id"`
		Message string `json:"message"`
	}{run.ID, run.Message})
	if err != nil {
		return store.SessionRunAdmission{}, err
	}
	var maxSeq int64
	for _, existing := range s.events {
		if existing.SessionID == run.SessionID && existing.Seq > maxSeq {
			maxSeq = existing.Seq
		}
	}
	queued := store.SessionEvent{ID: store.NewID(store.PrefixEvent), Type: "session.run.queued", Time: now, SessionID: run.SessionID, Seq: maxSeq + 1, DataJSON: queuedData, Data: queuedData}
	cp := copySessionRun(&run)
	s.runs[run.ID] = &cp
	ref := store.AggregateRef{Type: store.AggregateSession, ID: run.SessionID}
	s.aggregates[ref] = append(s.aggregates[ref], copyAggregateEvent(event))
	updated := copySession(session)
	updated.AggregateVersion = run.AdmittedVersion
	s.sessions[run.SessionID] = updated
	queuedCopy := copySessionEvent(&queued)
	s.events[queued.ID] = &queuedCopy
	return store.SessionRunAdmission{Run: copySessionRun(&cp), SessionVersion: run.AdmittedVersion, Created: true, QueuedEvent: queuedCopy}, nil
}

func (s *Store) ClaimNextSessionRun(ctx context.Context, sessionID string) (*store.SessionRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next *store.SessionRun
	for _, run := range s.runs {
		if run.SessionID == sessionID && run.Status == store.SessionRunStatusQueued && (next == nil || run.Sequence < next.Sequence) {
			next = run
		}
	}
	if next == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	next.Status = store.SessionRunStatusRunning
	next.StartedAt = now
	next.UpdatedAt = now
	cp := copySessionRun(next)
	return &cp, nil
}

func (s *Store) CompleteSessionRun(ctx context.Context, id, status, errorMessage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return store.ErrSessionNotFound
	}
	now := time.Now().UTC()
	run.Status = status
	run.ErrorMessage = errorMessage
	run.CompletedAt = now
	run.UpdatedAt = now
	return nil
}

func (s *Store) ListQueuedSessionRunSessions(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, run := range s.runs {
		if run.Status == store.SessionRunStatusQueued {
			seen[run.SessionID] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) AbortRunningSessionRuns(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, run := range s.runs {
		if run.Status == store.SessionRunStatusRunning {
			run.Status = store.SessionRunStatusAborted
			run.ErrorMessage = "server shutdown"
			run.CompletedAt = now
			run.UpdatedAt = now
		}
	}
	return nil
}

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

func copySessionRun(run *store.SessionRun) store.SessionRun {
	cp := *run
	cp.Agent = *copyAgent(&run.Agent)
	cp.OutputSchemaJSON = append([]byte(nil), run.OutputSchemaJSON...)
	return cp
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

func copyWorkspace(workspace *store.Workspace) *store.Workspace {
	if workspace == nil {
		return nil
	}
	cp := *workspace
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

func copyModelCall(c *store.ModelCall) store.ModelCall {
	cp := *c
	if c.StructuredOutputJSON != nil {
		cp.StructuredOutputJSON = make([]byte, len(c.StructuredOutputJSON))
		copy(cp.StructuredOutputJSON, c.StructuredOutputJSON)
	}
	if c.MetadataJSON != nil {
		cp.MetadataJSON = make([]byte, len(c.MetadataJSON))
		copy(cp.MetadataJSON, c.MetadataJSON)
	}
	if c.Trace != nil {
		cp.Trace = make([]byte, len(c.Trace))
		copy(cp.Trace, c.Trace)
	}
	return cp
}

func copySessionEvent(e *store.SessionEvent) store.SessionEvent {
	cp := *e
	if e.DataJSON != nil {
		cp.DataJSON = make([]byte, len(e.DataJSON))
		copy(cp.DataJSON, e.DataJSON)
		cp.Data = cp.DataJSON
	}
	return cp
}

func copyAggregateEvent(e store.AggregateEvent) store.AggregateEvent {
	e.Data = append(json.RawMessage(nil), e.Data...)
	return e
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

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("client name is required")
	}
	if strings.EqualFold(name, store.DefaultClientName) {
		return nil, store.ErrClientNameExists
	}
	for _, existing := range s.clients {
		if strings.EqualFold(existing.Name, name) {
			return nil, store.ErrClientNameExists
		}
	}

	client := &store.Client{
		ID:        store.NewID(store.PrefixClient),
		Name:      name,
		CreatedAt: store.Now(),
	}
	s.clients[client.ID] = copyClient(client)
	return client, nil
}

func (s *Store) EnsureDefaultClient() (*store.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, ok := s.clients[store.DefaultClientID]; ok {
		return copyClient(client), nil
	}
	client := &store.Client{
		ID:        store.DefaultClientID,
		Name:      store.DefaultClientName,
		CreatedAt: store.Now(),
	}
	s.clients[client.ID] = copyClient(client)
	return copyClient(client), nil
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

// ---- workspaces ---------------------------------------------------------------

func (s *Store) CreateWorkspace(workspace *store.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if workspace.ID == "" {
		workspace.ID = store.NewID(store.PrefixWorkspace)
	}
	now := store.Now()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	if workspace.ClientID != "" {
		if _, ok := s.clients[workspace.ClientID]; !ok {
			return fmt.Errorf("client not found: %s", workspace.ClientID)
		}
	}
	if s.workspaceNameExists(workspace.ClientID, workspace.Name, "") {
		return store.ErrWorkspaceNameExists
	}

	s.workspaces[workspace.ID] = copyWorkspace(workspace)
	return nil
}

func (s *Store) GetWorkspace(id string) (*store.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workspace, ok := s.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return copyWorkspace(workspace), nil
}

func (s *Store) ListWorkspaces() ([]*store.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Workspace, 0, len(s.workspaces))
	for _, workspace := range s.workspaces {
		out = append(out, copyWorkspace(workspace))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) ListWorkspacesByClient(clientID string) ([]*store.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Workspace, 0)
	for _, workspace := range s.workspaces {
		if workspace.ClientID == clientID {
			out = append(out, copyWorkspace(workspace))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) UpdateWorkspace(workspace *store.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.workspaces[workspace.ID]
	if !ok {
		return fmt.Errorf("workspace not found: %s", workspace.ID)
	}
	if workspace.ClientID != "" {
		if _, ok := s.clients[workspace.ClientID]; !ok {
			return fmt.Errorf("client not found: %s", workspace.ClientID)
		}
	}
	if s.workspaceNameExists(workspace.ClientID, workspace.Name, workspace.ID) {
		return store.ErrWorkspaceNameExists
	}

	workspace.UpdatedAt = store.Now()
	workspace.CreatedAt = existing.CreatedAt
	s.workspaces[workspace.ID] = copyWorkspace(workspace)
	return nil
}

func (s *Store) workspaceNameExists(clientID, name, excludeID string) bool {
	for _, existing := range s.workspaces {
		if existing.ID != excludeID && existing.ClientID == clientID && strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}

func (s *Store) DeleteWorkspace(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.workspaces[id]; !ok {
		return fmt.Errorf("workspace not found: %s", id)
	}
	delete(s.workspaces, id)
	for _, sess := range s.sessions {
		if sess.WorkspaceID == id {
			sess.WorkspaceID = ""
		}
	}
	return nil
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
	if session.WorkspaceID != "" {
		if _, ok := s.workspaces[session.WorkspaceID]; !ok {
			return fmt.Errorf("workspace not found: %s", session.WorkspaceID)
		}
	}
	ref := store.AggregateRef{Type: store.AggregateSession, ID: session.ID}
	if events := s.aggregates[ref]; len(events) != 0 {
		return &store.AggregateVersionConflict{Aggregate: ref, Expected: 0, Actual: int64(len(events))}
	}
	event, err := store.NewSessionCreatedEvent(*session)
	if err != nil {
		return err
	}
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	event.Version = 1
	projected, err := store.ProjectSession([]store.AggregateEvent{event})
	if err != nil {
		return err
	}
	s.aggregates[ref] = []store.AggregateEvent{copyAggregateEvent(event)}
	s.sessions[session.ID] = copySession(projected)
	*session = *copySession(projected)
	return nil
}

// ListAggregateEvents returns an aggregate's immutable events in version order.
func (s *Store) ListAggregateEvents(_ context.Context, aggregate store.AggregateRef, afterVersion int64, limit int) ([]store.AggregateEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	events := s.aggregates[aggregate]
	out := make([]store.AggregateEvent, 0, min(limit, len(events)))
	for _, event := range events {
		if event.Version <= afterVersion {
			continue
		}
		out = append(out, copyAggregateEvent(event))
		if len(out) == limit {
			break
		}
	}
	return out, nil
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

func (s *Store) ListSessionsByWorkspace(workspaceID string) ([]*store.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*store.Session, 0)
	for _, sess := range s.sessions {
		if sess.WorkspaceID == workspaceID {
			out = append(out, copySession(sess))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (s *Store) RenameSession(ctx context.Context, id, title string, expectedVersion int64) (*store.Session, error) {
	event, err := store.NewSessionRenamedEvent(id, title, store.Now())
	if err != nil {
		return nil, err
	}
	return s.applySessionMetadataEvent(ctx, event, expectedVersion, "")
}

func (s *Store) MoveSession(ctx context.Context, id, workDir, workspaceID string, expectedVersion int64) (*store.Session, error) {
	event, err := store.NewSessionMovedEvent(id, workDir, workspaceID, store.Now())
	if err != nil {
		return nil, err
	}
	return s.applySessionMetadataEvent(ctx, event, expectedVersion, workspaceID)
}

func (s *Store) applySessionMetadataEvent(_ context.Context, event store.AggregateEvent, expectedVersion int64, workspaceID string) (*store.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.sessions[event.Aggregate.ID]
	if !ok {
		return nil, store.ErrSessionNotFound
	}
	if existing.AggregateVersion != expectedVersion {
		return nil, &store.AggregateVersionConflict{Aggregate: event.Aggregate, Expected: expectedVersion, Actual: existing.AggregateVersion}
	}
	event.ClientID = existing.ClientID
	if workspaceID != "" {
		if _, ok := s.workspaces[workspaceID]; !ok {
			return nil, fmt.Errorf("workspace not found: %s", workspaceID)
		}
	}
	ref := event.Aggregate
	events := s.aggregates[ref]
	candidate := append(make([]store.AggregateEvent, 0, len(events)+1), events...)
	event.Version = expectedVersion + 1
	candidate = append(candidate, event)
	projected, err := store.ProjectSession(candidate)
	if err != nil {
		return nil, err
	}
	if event.Type == store.EventSessionRenamed && projected.Title == existing.Title {
		return copySession(existing), nil
	}
	if event.Type == store.EventSessionMoved && projected.WorkDir == existing.WorkDir && projected.WorkspaceID == existing.WorkspaceID {
		return copySession(existing), nil
	}
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	s.aggregates[ref] = append(events, copyAggregateEvent(event))
	s.sessions[ref.ID] = copySession(projected)
	return copySession(projected), nil
}

func (s *Store) PurgeSession(_ context.Context, id string, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return store.ErrSessionNotFound
	}
	ref := store.AggregateRef{Type: store.AggregateSession, ID: id}
	if session.AggregateVersion != expectedVersion {
		return &store.AggregateVersionConflict{Aggregate: ref, Expected: expectedVersion, Actual: session.AggregateVersion}
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
	for callID, call := range s.modelCalls {
		if call.SessionID == id {
			delete(s.modelCalls, callID)
		}
	}
	for eventID, event := range s.events {
		if event.SessionID == id {
			delete(s.events, eventID)
		}
	}
	for runID, run := range s.runs {
		if run.SessionID == id {
			delete(s.runs, runID)
		}
	}
	delete(s.aggregates, ref)
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

func (s *Store) UpsertModelCall(ctx context.Context, call store.ModelCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if call.ID == "" {
		call.ID = store.NewID(store.PrefixModelCall)
	}
	existing, exists := s.modelCalls[call.ID]
	if !exists {
		if _, ok := s.sessions[call.SessionID]; !ok {
			return store.ErrSessionNotFound
		}
		if call.RunID != "" {
			run, ok := s.runs[call.RunID]
			if !ok || run.SessionID != call.SessionID {
				return fmt.Errorf("session run %s does not belong to session %s", call.RunID, call.SessionID)
			}
		}
	}
	if call.Attempt == 0 {
		call.Attempt = 1
	}
	now := time.Now().UTC()
	if call.StartedAt.IsZero() {
		call.StartedAt = now
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = now
	}
	if call.UpdatedAt.IsZero() {
		call.UpdatedAt = now
	}
	if exists {
		call.SessionID = existing.SessionID
		call.RunID = existing.RunID
		call.Step = existing.Step
		call.Attempt = existing.Attempt
		call.StartedAt = existing.StartedAt
		call.CreatedAt = existing.CreatedAt
	} else if call.RunID != "" {
		for _, existing := range s.modelCalls {
			if existing.RunID == call.RunID && existing.Step == call.Step && existing.Attempt == call.Attempt {
				return store.ErrModelCallAttemptConflict
			}
		}
	}
	cp := copyModelCall(&call)
	s.modelCalls[call.ID] = &cp
	return nil
}

func (s *Store) LatestModelCall(ctx context.Context, sessionID string) (*store.ModelCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	var latest *store.ModelCall
	for _, call := range s.modelCalls {
		if call.SessionID != sessionID || call.ContextTokens == 0 {
			continue
		}
		if latest == nil || call.StartedAt.After(latest.StartedAt) || (call.StartedAt.Equal(latest.StartedAt) && call.ID > latest.ID) {
			latest = call
		}
	}
	if latest == nil {
		return nil, nil
	}
	cp := copyModelCall(latest)
	return &cp, nil
}

func (s *Store) ListModelCalls(ctx context.Context, sessionID string) ([]store.ModelCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	var out []store.ModelCall
	for _, call := range s.modelCalls {
		if call.SessionID == sessionID {
			out = append(out, copyModelCall(call))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	if out == nil {
		out = []store.ModelCall{}
	}
	return out, nil
}

func (s *Store) AppendSessionEvent(ctx context.Context, event store.SessionEvent) (store.SessionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[event.SessionID]; !ok {
		return store.SessionEvent{}, store.ErrSessionNotFound
	}
	if event.ID == "" {
		event.ID = store.NewID(store.PrefixEvent)
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if len(event.DataJSON) == 0 && len(event.Data) > 0 {
		event.DataJSON = []byte(event.Data)
	}
	if len(event.DataJSON) == 0 {
		event.DataJSON = []byte(`{}`)
	}
	var maxSeq int64
	for _, existing := range s.events {
		if existing.SessionID == event.SessionID && existing.Seq > maxSeq {
			maxSeq = existing.Seq
		}
	}
	event.Seq = maxSeq + 1
	event.Data = event.DataJSON
	cp := copySessionEvent(&event)
	s.events[event.ID] = &cp
	return copySessionEvent(&cp), nil
}

func (s *Store) ListSessionEvents(ctx context.Context, sessionID string, after int64, limit int) ([]store.SessionEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []store.SessionEvent{}
	for _, event := range s.events {
		if event.SessionID == sessionID && event.Seq > after {
			out = append(out, copySessionEvent(event))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
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
