// Package memory provides an in-memory implementation of store.Store
// suitable for ephemeral runs. It conforms to the same behavioral contract
// as store/sqlite.go.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/store"
)

// Store is an in-memory implementation of store.Store protected by a
// single sync.RWMutex.
type Store struct {
	mu                 sync.RWMutex
	agents             map[string]*store.Agent
	sessions           map[string]*store.Session
	clients            map[string]*store.Client
	workspaces         map[string]*store.Workspace
	messages           map[string]*store.StoredMessage
	parts              map[string]*store.StoredPart
	modelCalls         map[string]*store.ModelCall
	toolUses           map[string]*store.ToolUse
	permissionRequests map[string]*store.PermissionRequest
	permissionGrants   map[string]*store.PermissionGrant
	events             map[string]*store.SessionEvent
	aggregates         map[store.AggregateRef][]store.AggregateEvent
	globalSeq          int64
	runs               map[string]*store.SessionRun
	auth               *store.Auth
}

// NewStore returns a fresh empty in-memory store.
func NewStore() *Store {
	return &Store{
		agents:             make(map[string]*store.Agent),
		sessions:           make(map[string]*store.Session),
		clients:            make(map[string]*store.Client),
		workspaces:         make(map[string]*store.Workspace),
		messages:           make(map[string]*store.StoredMessage),
		parts:              make(map[string]*store.StoredPart),
		modelCalls:         make(map[string]*store.ModelCall),
		toolUses:           make(map[string]*store.ToolUse),
		permissionRequests: make(map[string]*store.PermissionRequest),
		permissionGrants:   make(map[string]*store.PermissionGrant),
		events:             make(map[string]*store.SessionEvent),
		aggregates:         make(map[store.AggregateRef][]store.AggregateEvent),
		runs:               make(map[string]*store.SessionRun),
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
	queued := store.SessionEvent{ID: store.NewID(store.PrefixEvent), SchemaVersion: 1, Type: "session.run.queued", Time: now, SessionID: run.SessionID, Seq: maxSeq + 1, DataJSON: queuedData, Data: queuedData}
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

func (s *Store) GetSessionRun(ctx context.Context, sessionID, runID string) (*store.SessionRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	run, ok := s.runs[runID]
	if !ok || run.SessionID != sessionID {
		return nil, store.ErrSessionRunNotFound
	}
	cp := copySessionRun(run)
	return &cp, nil
}

func (s *Store) ListSessionRuns(ctx context.Context, sessionID string) ([]store.SessionRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	out := []store.SessionRun{}
	for _, run := range s.runs {
		if run.SessionID == sessionID {
			out = append(out, copySessionRun(run))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, nil
}

func (s *Store) ClaimNextSessionRun(ctx context.Context, sessionID string) (store.SessionRunTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return store.SessionRunTransition{}, store.ErrSessionNotFound
	}
	for _, run := range s.runs {
		if run.SessionID == sessionID && run.Status == store.SessionRunStatusRunning {
			return store.SessionRunTransition{}, nil
		}
	}
	var next *store.SessionRun
	for _, run := range s.runs {
		if run.SessionID == sessionID && run.Status == store.SessionRunStatusQueued && (next == nil || run.Sequence < next.Sequence) {
			next = run
		}
	}
	if next == nil {
		return store.SessionRunTransition{}, nil
	}
	now := time.Now().UTC()
	candidate := copySessionRun(next)
	candidate.Status, candidate.StartedAt, candidate.UpdatedAt = store.SessionRunStatusRunning, now, now
	aggregateEvent, err := store.NewSessionRunTransitionEvent(candidate)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	events := s.aggregates[aggregateEvent.Aggregate]
	aggregateEvent.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), events...), aggregateEvent))
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	event, err := s.appendRunEventLocked(&candidate, "session.run.started", nil, now)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	s.globalSeq++
	aggregateEvent.GlobalSequence = s.globalSeq
	s.aggregates[aggregateEvent.Aggregate] = append(events, copyAggregateEvent(aggregateEvent))
	s.sessions[sessionID] = copySession(projected)
	*next = candidate
	return store.SessionRunTransition{Run: copySessionRun(next), Event: event, Changed: true}, nil
}

func (s *Store) SettleSessionRun(ctx context.Context, settlement store.SessionRunSettlement) (store.SessionRunTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !sessionRunTerminal(settlement.Status) {
		return store.SessionRunTransition{}, store.ErrSessionRunTransitionConflict
	}
	run, ok := s.runs[settlement.ID]
	if !ok {
		return store.SessionRunTransition{}, store.ErrSessionRunNotFound
	}
	if sessionRunTerminal(run.Status) {
		if run.Status == settlement.Status && run.ErrorType == settlement.ErrorType && run.ErrorMessage == settlement.ErrorMessage {
			return store.SessionRunTransition{Run: copySessionRun(run)}, nil
		}
		return store.SessionRunTransition{}, store.ErrSessionRunTransitionConflict
	}
	if run.Status != settlement.ExpectedStatus || !legalSessionRunSettlement(run.Status, settlement.Status) {
		return store.SessionRunTransition{}, store.ErrSessionRunTransitionConflict
	}
	now := time.Now().UTC()
	candidate := copySessionRun(run)
	candidate.Status, candidate.ErrorType, candidate.ErrorMessage = settlement.Status, settlement.ErrorType, settlement.ErrorMessage
	candidate.CompletedAt, candidate.UpdatedAt = now, now
	session, ok := s.sessions[candidate.SessionID]
	if !ok {
		return store.SessionRunTransition{}, store.ErrSessionNotFound
	}
	aggregateEvent, err := store.NewSessionRunTransitionEvent(candidate)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	events := s.aggregates[aggregateEvent.Aggregate]
	aggregateEvent.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), events...), aggregateEvent))
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	event, err := s.appendRunEventLocked(&candidate, "session.run."+settlement.Status, settlement.EventData, now)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	s.globalSeq++
	aggregateEvent.GlobalSequence = s.globalSeq
	s.aggregates[aggregateEvent.Aggregate] = append(events, copyAggregateEvent(aggregateEvent))
	s.sessions[candidate.SessionID] = copySession(projected)
	*run = candidate
	return store.SessionRunTransition{Run: copySessionRun(run), Event: event, Changed: true}, nil
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

func (s *Store) CountQueuedSessionRuns(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var count int
	for _, run := range s.runs {
		if run.Status == store.SessionRunStatusQueued {
			count++
		}
	}
	return count, nil
}

func (s *Store) ListRunningSessionRuns(ctx context.Context) ([]store.SessionRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []store.SessionRun{}
	for _, run := range s.runs {
		if run.Status == store.SessionRunStatusRunning {
			out = append(out, copySessionRun(run))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SessionID == out[j].SessionID {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func sessionRunTerminal(status string) bool {
	return status == store.SessionRunStatusCompleted || status == store.SessionRunStatusFailed || status == store.SessionRunStatusAborted
}
func legalSessionRunSettlement(from, to string) bool {
	return (from == store.SessionRunStatusRunning && sessionRunTerminal(to)) || (from == store.SessionRunStatusQueued && to == store.SessionRunStatusAborted)
}
func (s *Store) appendRunEventLocked(run *store.SessionRun, typ string, extra map[string]any, now time.Time) (store.SessionEvent, error) {
	data := make(map[string]any, len(extra)+7)
	for k, v := range extra {
		data[k] = v
	}
	data["run_id"], data["status"], data["error_type"], data["error_message"] = run.ID, run.Status, run.ErrorType, run.ErrorMessage
	if !run.StartedAt.IsZero() {
		data["started_at"] = run.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.CompletedAt.IsZero() {
		data["completed_at"] = run.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.UpdatedAt.IsZero() {
		data["updated_at"] = run.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return store.SessionEvent{}, err
	}
	var max int64
	for _, event := range s.events {
		if event.SessionID == run.SessionID && event.Seq > max {
			max = event.Seq
		}
	}
	event := store.SessionEvent{ID: store.NewID(store.PrefixEvent), SchemaVersion: 1, Type: typ, Time: now, SessionID: run.SessionID, Seq: max + 1, DataJSON: payload, Data: payload}
	cp := copySessionEvent(&event)
	s.events[event.ID] = &cp
	return cp, nil
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
	if encoded, err := json.Marshal(a); err == nil {
		var copied store.Agent
		if json.Unmarshal(encoded, &copied) == nil {
			return &copied
		}
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

func copyToolUse(use *store.ToolUse) store.ToolUse {
	cp := *use
	cp.InputJSON = append([]byte(nil), use.InputJSON...)
	cp.StructuredJSON = append([]byte(nil), use.StructuredJSON...)
	cp.MetadataJSON = append([]byte(nil), use.MetadataJSON...)
	return cp
}

func copyPermissionRequest(request *store.PermissionRequest) store.PermissionRequest {
	cp := *request
	cp.Resources = append([]string(nil), request.Resources...)
	return cp
}

func copyPermissionGrant(grant *store.PermissionGrant) store.PermissionGrant { return *grant }

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
	return s.CreateClientWithID(store.NewID(store.PrefixClient), name)
}

func (s *Store) CreateClientWithID(id, name string) (*store.Client, error) {
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
	if _, ok := s.clients[id]; ok {
		return nil, store.ErrClientIDExists
	}

	client := &store.Client{
		ID:        id,
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

// RebuildSessionProjections replaces one Session aggregate's derived state
// from immutable aggregate history. Public session events remain untouched.
func (s *Store) RebuildSessionProjections(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := store.AggregateRef{Type: store.AggregateSession, ID: sessionID}
	projection, err := store.ProjectSessionAggregate(copyAggregateEvents(s.aggregates[ref]))
	if err != nil {
		return fmt.Errorf("rebuild session projections %s: %w", sessionID, err)
	}
	s.replaceSessionProjectionLocked(projection)
	return nil
}

// RebuildAllSessionProjections replaces all Session aggregate projections only
// after every aggregate history has been successfully validated and copied.
func (s *Store) RebuildAllSessionProjections(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	projections := make([]store.SessionAggregateProjection, 0)
	for ref, events := range s.aggregates {
		if ref.Type != store.AggregateSession {
			continue
		}
		projection, err := store.ProjectSessionAggregate(copyAggregateEvents(events))
		if err != nil {
			return fmt.Errorf("rebuild session projections %s: %w", ref.ID, err)
		}
		projections = append(projections, projection)
	}
	for _, projection := range projections {
		s.replaceSessionProjectionLocked(projection)
	}
	return nil
}

func (s *Store) replaceSessionProjectionLocked(projection store.SessionAggregateProjection) {
	id := projection.Session.ID
	for key, value := range s.runs {
		if value.SessionID == id {
			delete(s.runs, key)
		}
	}
	for key, value := range s.parts {
		if message, ok := s.messages[value.MessageID]; ok && message.SessionID == id {
			delete(s.parts, key)
		}
	}
	for key, value := range s.messages {
		if value.SessionID == id {
			delete(s.messages, key)
		}
	}
	for key, value := range s.modelCalls {
		if value.SessionID == id {
			delete(s.modelCalls, key)
		}
	}
	for key, value := range s.toolUses {
		if value.SessionID == id {
			delete(s.toolUses, key)
		}
	}
	for key, value := range s.permissionRequests {
		if value.SessionID == id {
			delete(s.permissionRequests, key)
		}
	}
	for key, value := range s.permissionGrants {
		if value.SessionID == id {
			delete(s.permissionGrants, key)
		}
	}
	s.sessions[id] = copySession(projection.Session)
	for _, run := range projection.Runs {
		cp := copySessionRun(&run)
		s.runs[run.ID] = &cp
	}
	for _, message := range projection.Messages {
		cp := copyMessage(&message)
		cp.Parts = nil
		s.messages[message.ID] = &cp
		for _, part := range message.Parts {
			partCopy := copyPart(&part)
			s.parts[part.ID] = &partCopy
		}
	}
	for _, call := range projection.ModelCalls {
		cp := copyModelCall(&call)
		s.modelCalls[call.ID] = &cp
	}
	for _, use := range projection.ToolUses {
		cp := copyToolUse(&use)
		s.toolUses[use.ID] = &cp
	}
	for _, request := range projection.PermissionRequests {
		cp := copyPermissionRequest(&request)
		s.permissionRequests[request.ID] = &cp
	}
	for _, grant := range projection.PermissionGrants {
		cp := copyPermissionGrant(&grant)
		s.permissionGrants[grant.ID] = &cp
	}
}

func copyAggregateEvents(events []store.AggregateEvent) []store.AggregateEvent {
	copied := make([]store.AggregateEvent, len(events))
	for i, event := range events {
		copied[i] = copyAggregateEvent(event)
	}
	return copied
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
	for useID, use := range s.toolUses {
		if use.SessionID == id {
			delete(s.toolUses, useID)
		}
	}
	for requestID, request := range s.permissionRequests {
		if request.SessionID == id {
			delete(s.permissionRequests, requestID)
		}
	}
	for grantID, grant := range s.permissionGrants {
		if grant.SessionID == id {
			delete(s.permissionGrants, grantID)
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

func (s *Store) SaveMessage(ctx context.Context, msg store.StoredMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.Revision == 0 {
		msg.Revision = 1
	}
	if msg.State == "" {
		msg.State = "completed"
	}
	if _, ok := s.sessions[msg.SessionID]; !ok {
		return store.ErrSessionNotFound
	}
	if err := validateMessageParts(msg); err != nil {
		return err
	}
	if existing, ok := s.messages[msg.ID]; ok {
		if existing.SessionID != msg.SessionID || existing.RunID != msg.RunID || existing.Idx != msg.Idx || existing.Role != msg.Role {
			return fmt.Errorf("message identity is immutable")
		}
		if msg.Revision < existing.Revision {
			return store.ErrMessageRevisionStale
		}
		existingParts := messageParts(s.parts, msg.ID)
		if msg.Revision == existing.Revision {
			if messageRevisionEqual(*existing, existingParts, msg) {
				return nil
			}
			return store.ErrMessageRevisionConflict
		}
		snapshot := prepareMessageSnapshot(msg, existing, existingParts, time.Now().UTC())
		if err := s.appendMessageAggregateLocked(snapshot); err != nil {
			return err
		}
		s.applyMessageSnapshotLocked(snapshot)
		return nil
	}
	for _, existing := range s.messages {
		if existing.SessionID == msg.SessionID && existing.Idx == msg.Idx {
			return fmt.Errorf("message index belongs to %s", existing.ID)
		}
	}
	if msg.RunID != "" {
		run, ok := s.runs[msg.RunID]
		if !ok || run.SessionID != msg.SessionID {
			return fmt.Errorf("session run %s does not belong to session %s", msg.RunID, msg.SessionID)
		}
	}
	for _, part := range msg.Parts {
		if owner, ok := s.parts[part.ID]; ok && owner.MessageID != msg.ID {
			return fmt.Errorf("part %s belongs to message %s", part.ID, owner.MessageID)
		}
	}
	snapshot := prepareMessageSnapshot(msg, nil, nil, time.Now().UTC())
	if err := s.appendMessageAggregateLocked(snapshot); err != nil {
		return err
	}
	s.applyMessageSnapshotLocked(snapshot)
	return nil
}

func validateMessageParts(msg store.StoredMessage) error {
	ids := make(map[string]struct{}, len(msg.Parts))
	sequences := make(map[int]struct{}, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.MessageID != msg.ID {
			return fmt.Errorf("part %s does not belong to message %s", part.ID, msg.ID)
		}
		if _, ok := ids[part.ID]; ok {
			return fmt.Errorf("duplicate part ID %s", part.ID)
		}
		if _, ok := sequences[part.Sequence]; ok {
			return fmt.Errorf("duplicate part sequence %d", part.Sequence)
		}
		ids[part.ID] = struct{}{}
		sequences[part.Sequence] = struct{}{}
	}
	return nil
}

func messageParts(parts map[string]*store.StoredPart, messageID string) []store.StoredPart {
	out := make([]store.StoredPart, 0)
	for _, part := range parts {
		if part.MessageID == messageID {
			out = append(out, copyPart(part))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

func messageRevisionEqual(existing store.StoredMessage, existingParts []store.StoredPart, incoming store.StoredMessage) bool {
	if existing.Role != incoming.Role || existing.State != incoming.State || !bytes.Equal(existing.MetadataJSON, incoming.MetadataJSON) || len(existingParts) != len(incoming.Parts) {
		return false
	}
	for i := range existingParts {
		a, b := existingParts[i], incoming.Parts[i]
		if a.ID != b.ID || a.Sequence != b.Sequence || a.Kind != b.Kind || !bytes.Equal(a.PayloadJSON, b.PayloadJSON) {
			return false
		}
	}
	return true
}

func prepareMessageSnapshot(msg store.StoredMessage, existing *store.StoredMessage, oldParts []store.StoredPart, now time.Time) store.StoredMessage {
	if existing == nil {
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = now
		}
	} else {
		msg.CreatedAt = existing.CreatedAt
	}
	if msg.UpdatedAt.IsZero() {
		msg.UpdatedAt = now
	}
	oldByID := make(map[string]store.StoredPart, len(oldParts))
	for _, part := range oldParts {
		oldByID[part.ID] = part
	}
	for i, part := range msg.Parts {
		if old, ok := oldByID[part.ID]; ok {
			part.CreatedAt = old.CreatedAt
		} else if part.CreatedAt.IsZero() {
			part.CreatedAt = now
		}
		if part.UpdatedAt.IsZero() {
			part.UpdatedAt = now
		}
		msg.Parts[i] = part
	}
	return msg
}

func (s *Store) appendMessageAggregateLocked(message store.StoredMessage) error {
	session := s.sessions[message.SessionID]
	event, err := store.NewSessionMessageSavedEvent(message)
	if err != nil {
		return err
	}
	events := s.aggregates[event.Aggregate]
	event.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), events...), event))
	if err != nil {
		return err
	}
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	s.aggregates[event.Aggregate] = append(events, copyAggregateEvent(event))
	s.sessions[message.SessionID] = copySession(projected)
	return nil
}

func (s *Store) applyMessageSnapshotLocked(message store.StoredMessage) {
	stored := copyMessage(&message)
	stored.Parts = nil
	s.messages[message.ID] = &stored
	for id, part := range s.parts {
		if part.MessageID == message.ID {
			delete(s.parts, id)
		}
	}
	for _, part := range message.Parts {
		cp := copyPart(&part)
		s.parts[part.ID] = &cp
	}
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
	event, err := store.NewSessionModelCallSavedEvent(call)
	if err != nil {
		return err
	}
	session := s.sessions[call.SessionID]
	events := s.aggregates[event.Aggregate]
	event.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), events...), event))
	if err != nil {
		return err
	}
	cp := copyModelCall(&call)
	s.modelCalls[call.ID] = &cp
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	s.aggregates[event.Aggregate] = append(events, copyAggregateEvent(event))
	s.sessions[call.SessionID] = copySession(projected)
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

func (s *Store) InterruptActiveModelCalls(ctx context.Context, runID, errorType, errorMessage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	type update struct {
		call      *store.ModelCall
		candidate store.ModelCall
		event     store.AggregateEvent
		projected *store.Session
	}
	updates := []update{}
	pendingEvents := make(map[store.AggregateRef][]store.AggregateEvent)
	pendingSessions := make(map[string]*store.Session)
	for _, call := range s.modelCalls {
		if call.RunID == runID && call.Status == store.ModelCallStatusStarted {
			candidate := copyModelCall(call)
			candidate.Status, candidate.ErrorType, candidate.ErrorMessage = store.ModelCallStatusAborted, errorType, errorMessage
			candidate.CompletedAt, candidate.UpdatedAt = now, now
			event, err := store.NewSessionModelCallSavedEvent(candidate)
			if err != nil {
				return err
			}
			session := pendingSessions[candidate.SessionID]
			if session == nil {
				session = s.sessions[candidate.SessionID]
			}
			events, ok := pendingEvents[event.Aggregate]
			if !ok {
				events = append([]store.AggregateEvent(nil), s.aggregates[event.Aggregate]...)
			}
			event.Version = session.AggregateVersion + 1
			projected, err := store.ProjectSession(append(events, event))
			if err != nil {
				return err
			}
			pendingEvents[event.Aggregate] = append(events, event)
			pendingSessions[candidate.SessionID] = projected
			updates = append(updates, update{call: call, candidate: candidate, event: event, projected: projected})
		}
	}
	for _, update := range updates {
		*update.call = update.candidate
		s.globalSeq++
		update.event.GlobalSequence = s.globalSeq
		ref := update.event.Aggregate
		// pending events hold the validated sequence; append the committed copy one at a time.
		s.aggregates[ref] = append(s.aggregates[ref], copyAggregateEvent(update.event))
		s.sessions[update.candidate.SessionID] = copySession(update.projected)
	}
	return nil
}

func (s *Store) CreatePermissionRequest(ctx context.Context, request store.PermissionRequest) (store.PermissionRequestTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request.Status == "" {
		request.Status = store.PermissionRequestStatusPending
	}
	if request.Status != store.PermissionRequestStatusPending || request.Action == "" || len(request.Resources) == 0 {
		return store.PermissionRequestTransition{}, &store.PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
	}
	if _, ok := s.sessions[request.SessionID]; !ok {
		return store.PermissionRequestTransition{}, store.ErrSessionNotFound
	}
	if request.RunID != "" {
		run, ok := s.runs[request.RunID]
		if !ok || run.SessionID != request.SessionID {
			return store.PermissionRequestTransition{}, fmt.Errorf("session run %s does not belong to session %s", request.RunID, request.SessionID)
		}
	}
	if request.ToolUseID != "" {
		use, ok := s.toolUses[request.ToolUseID]
		if !ok || use.SessionID != request.SessionID {
			return store.PermissionRequestTransition{}, fmt.Errorf("tool use %s does not belong to session %s", request.ToolUseID, request.SessionID)
		}
		if request.RunID != "" && use.RunID != request.RunID {
			return store.PermissionRequestTransition{}, fmt.Errorf("tool use %s does not belong to run %s", request.ToolUseID, request.RunID)
		}
		if request.CallID != "" && use.CallID != "" && use.CallID != request.CallID {
			return store.PermissionRequestTransition{}, fmt.Errorf("tool use %s does not belong to call %s", request.ToolUseID, request.CallID)
		}
	}
	for _, resource := range request.Resources {
		if resource == "" {
			return store.PermissionRequestTransition{}, &store.PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
		}
	}
	if request.ID == "" {
		request.ID = store.NewID(store.PrefixPermissionRequest)
	}
	if existing, exists := s.permissionRequests[request.ID]; exists {
		if samePendingPermissionRequestMemory(*existing, request) {
			return store.PermissionRequestTransition{Request: copyPermissionRequest(existing)}, nil
		}
		return store.PermissionRequestTransition{}, &store.PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
	}
	now := time.Now().UTC()
	request.CreatedAt, request.UpdatedAt = now, now
	event, err := s.newPermissionEventLocked(&request, "session.permission.requested", now)
	if err != nil {
		return store.PermissionRequestTransition{}, err
	}
	aggregateEvent, projected, err := s.permissionRequestAggregateLocked(s.sessions[request.SessionID], s.aggregates[store.AggregateRef{Type: store.AggregateSession, ID: request.SessionID}], request, false)
	if err != nil {
		return store.PermissionRequestTransition{}, err
	}
	cp := copyPermissionRequest(&request)
	s.permissionRequests[request.ID] = &cp
	s.appendPermissionAggregateCommitLocked(aggregateEvent, projected)
	eventCopy := copySessionEvent(&event)
	s.events[event.ID] = &eventCopy
	return store.PermissionRequestTransition{Request: cp, Event: eventCopy, Changed: true}, nil
}

func (s *Store) GetPermissionRequest(ctx context.Context, sessionID, requestID string) (*store.PermissionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	request, ok := s.permissionRequests[requestID]
	if !ok || request.SessionID != sessionID {
		return nil, &store.PermissionRequestNotFound{SessionID: sessionID, RequestID: requestID}
	}
	cp := copyPermissionRequest(request)
	return &cp, nil
}

func (s *Store) ListPermissionRequests(ctx context.Context, sessionID string) ([]store.PermissionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	out := []store.PermissionRequest{}
	for _, request := range s.permissionRequests {
		if request.SessionID == sessionID {
			out = append(out, copyPermissionRequest(request))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) ResolvePermissionRequest(ctx context.Context, resolution store.PermissionRequestResolution) (store.PermissionRequestTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resolution.ExpectedStatus == "" {
		resolution.ExpectedStatus = store.PermissionRequestStatusPending
	}
	if resolution.ExpectedStatus != store.PermissionRequestStatusPending || !legalPermissionResolutionMemory(resolution.Status, resolution.Response) {
		return store.PermissionRequestTransition{}, &store.PermissionRequestTransitionConflict{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	if _, ok := s.sessions[resolution.SessionID]; !ok {
		return store.PermissionRequestTransition{}, store.ErrSessionNotFound
	}
	request, ok := s.permissionRequests[resolution.RequestID]
	if !ok || request.SessionID != resolution.SessionID {
		return store.PermissionRequestTransition{}, &store.PermissionRequestNotFound{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	if request.Status != store.PermissionRequestStatusPending {
		if request.Status == resolution.Status && request.Response == resolution.Response && request.ErrorType == resolution.ErrorType && request.ErrorMessage == resolution.ErrorMessage {
			return store.PermissionRequestTransition{Request: copyPermissionRequest(request)}, nil
		}
		return store.PermissionRequestTransition{}, &store.PermissionRequestTransitionConflict{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	now := time.Now().UTC()
	updated := copyPermissionRequest(request)
	updated.Status, updated.Response, updated.ErrorType, updated.ErrorMessage = resolution.Status, resolution.Response, resolution.ErrorType, resolution.ErrorMessage
	updated.ResolvedAt, updated.UpdatedAt = now, now
	event, err := s.newPermissionEventLocked(&updated, "session.permission.resolved", now)
	if err != nil {
		return store.PermissionRequestTransition{}, err
	}
	aggregateEvents := []store.AggregateEvent{}
	projected := s.sessions[updated.SessionID]
	aggregateEvent, projected, err := s.permissionRequestAggregateLocked(projected, s.aggregates[store.AggregateRef{Type: store.AggregateSession, ID: updated.SessionID}], updated, true)
	if err != nil {
		return store.PermissionRequestTransition{}, err
	}
	aggregateEvents = append(aggregateEvents, aggregateEvent)
	history := append(append([]store.AggregateEvent(nil), s.aggregates[aggregateEvent.Aggregate]...), aggregateEvent)
	grants := []store.PermissionGrant{}
	if updated.Response == store.PermissionResponseAlways {
		for _, resource := range updated.Resources {
			found := false
			for _, grant := range s.permissionGrants {
				if grant.SessionID == updated.SessionID && grant.Action == updated.Action && grant.Resource == resource {
					found = true
					break
				}
			}
			if !found {
				grant := store.PermissionGrant{ID: store.NewID(store.PrefixPermissionGrant), SessionID: updated.SessionID, Action: updated.Action, Resource: resource, CreatedAt: now}
				aggregateEvent, next, err := s.permissionGrantAggregateLocked(projected, history, grant)
				if err != nil {
					return store.PermissionRequestTransition{}, err
				}
				projected = next
				aggregateEvents = append(aggregateEvents, aggregateEvent)
				history = append(history, aggregateEvent)
				grants = append(grants, grant)
			}
		}
	}
	for _, aggregateEvent := range aggregateEvents {
		s.appendPermissionAggregateCommitLocked(aggregateEvent, projected)
	}
	for _, grant := range grants {
		cp := copyPermissionGrant(&grant)
		s.permissionGrants[grant.ID] = &cp
	}
	*request = updated
	eventCopy := copySessionEvent(&event)
	s.events[event.ID] = &eventCopy
	return store.PermissionRequestTransition{Request: copyPermissionRequest(request), Event: eventCopy, Changed: true}, nil
}

func (s *Store) ListPermissionGrants(ctx context.Context, sessionID string) ([]store.PermissionGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	out := []store.PermissionGrant{}
	for _, grant := range s.permissionGrants {
		if grant.SessionID == sessionID {
			out = append(out, copyPermissionGrant(grant))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) InterruptPendingPermissionRequests(ctx context.Context) ([]store.PermissionRequestTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	out := []store.PermissionRequestTransition{}
	requests := make([]*store.PermissionRequest, 0)
	for _, request := range s.permissionRequests {
		if request.Status == store.PermissionRequestStatusPending {
			requests = append(requests, request)
		}
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].CreatedAt.Equal(requests[j].CreatedAt) {
			return requests[i].ID < requests[j].ID
		}
		return requests[i].CreatedAt.Before(requests[j].CreatedAt)
	})
	type pendingTransition struct {
		request        *store.PermissionRequest
		updated        store.PermissionRequest
		event          store.SessionEvent
		aggregateEvent store.AggregateEvent
		projected      *store.Session
	}
	pending := make([]pendingTransition, 0, len(requests))
	pendingSessions := make(map[string]*store.Session)
	pendingHistories := make(map[store.AggregateRef][]store.AggregateEvent)
	for _, request := range requests {
		updated := copyPermissionRequest(request)
		updated.Status, updated.ErrorType, updated.ErrorMessage = store.PermissionRequestStatusInterrupted, "process_interrupted", "permission request interrupted because the process stopped"
		updated.ResolvedAt, updated.UpdatedAt = now, now
		event, err := s.newPermissionEventLocked(&updated, "session.permission.resolved", now)
		if err != nil {
			return nil, err
		}
		ref := store.AggregateRef{Type: store.AggregateSession, ID: updated.SessionID}
		session := pendingSessions[updated.SessionID]
		if session == nil {
			session = s.sessions[updated.SessionID]
			pendingHistories[ref] = append([]store.AggregateEvent(nil), s.aggregates[ref]...)
		}
		aggregateEvent, projected, err := s.permissionRequestAggregateLocked(session, pendingHistories[ref], updated, true)
		if err != nil {
			return nil, err
		}
		pendingSessions[updated.SessionID] = projected
		pendingHistories[ref] = append(pendingHistories[ref], aggregateEvent)
		pending = append(pending, pendingTransition{request: request, updated: updated, event: event, aggregateEvent: aggregateEvent, projected: projected})
	}
	for _, transition := range pending {
		s.appendPermissionAggregateCommitLocked(transition.aggregateEvent, transition.projected)
		*transition.request = transition.updated
		eventCopy := copySessionEvent(&transition.event)
		s.events[eventCopy.ID] = &eventCopy
		out = append(out, store.PermissionRequestTransition{Request: copyPermissionRequest(transition.request), Event: eventCopy, Changed: true})
	}
	return out, nil
}

func legalPermissionResolutionMemory(status, response string) bool {
	return (response == store.PermissionResponseOnce || response == store.PermissionResponseAlways) && status == store.PermissionRequestStatusApproved || response == store.PermissionResponseReject && status == store.PermissionRequestStatusRejected || response == "" && (status == store.PermissionRequestStatusTimedOut || status == store.PermissionRequestStatusInterrupted)
}

func samePendingPermissionRequestMemory(existing, request store.PermissionRequest) bool {
	return existing.SessionID == request.SessionID && existing.RunID == request.RunID && existing.ToolUseID == request.ToolUseID && existing.CallID == request.CallID && existing.Action == request.Action && existing.Status == store.PermissionRequestStatusPending && request.Status == store.PermissionRequestStatusPending && slices.Equal(existing.Resources, request.Resources)
}

func (s *Store) newPermissionEventLocked(request *store.PermissionRequest, typ string, now time.Time) (store.SessionEvent, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return store.SessionEvent{}, err
	}
	var max int64
	for _, event := range s.events {
		if event.SessionID == request.SessionID && event.Seq > max {
			max = event.Seq
		}
	}
	event := store.SessionEvent{ID: store.NewID(store.PrefixEvent), SchemaVersion: 1, Type: typ, Time: now, SessionID: request.SessionID, Seq: max + 1, DataJSON: payload, Data: payload}
	return event, nil
}

func (s *Store) permissionRequestAggregateLocked(session *store.Session, history []store.AggregateEvent, request store.PermissionRequest, resolved bool) (store.AggregateEvent, *store.Session, error) {
	var (
		event store.AggregateEvent
		err   error
	)
	if resolved {
		event, err = store.NewSessionPermissionResolutionEvent(request)
	} else {
		event, err = store.NewSessionPermissionRequestEvent(request)
	}
	if err != nil {
		return store.AggregateEvent{}, nil, err
	}
	event.ClientID = session.ClientID
	event.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), history...), event))
	if err != nil {
		return store.AggregateEvent{}, nil, err
	}
	return event, projected, nil
}

func (s *Store) permissionGrantAggregateLocked(session *store.Session, history []store.AggregateEvent, grant store.PermissionGrant) (store.AggregateEvent, *store.Session, error) {
	event, err := store.NewSessionPermissionGrantCreatedEvent(grant)
	if err != nil {
		return store.AggregateEvent{}, nil, err
	}
	event.ClientID = session.ClientID
	event.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), history...), event))
	if err != nil {
		return store.AggregateEvent{}, nil, err
	}
	return event, projected, nil
}

func (s *Store) appendPermissionAggregateCommitLocked(event store.AggregateEvent, projected *store.Session) {
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	s.aggregates[event.Aggregate] = append(s.aggregates[event.Aggregate], copyAggregateEvent(event))
	s.sessions[event.Aggregate.ID] = copySession(projected)
}

func (s *Store) SaveToolUse(ctx context.Context, use store.ToolUse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if use.ID == "" {
		use.ID = store.NewID(store.PrefixToolUse)
	}
	existing, exists := s.toolUses[use.ID]
	if !exists {
		if _, ok := s.sessions[use.SessionID]; !ok {
			return store.ErrSessionNotFound
		}
		if use.Status != store.ToolUseStatusProposed {
			return store.ErrToolUseInvalidTransition
		}
		if use.RunID != "" {
			run, ok := s.runs[use.RunID]
			if !ok || run.SessionID != use.SessionID {
				return fmt.Errorf("session run %s does not belong to session %s", use.RunID, use.SessionID)
			}
			for _, other := range s.toolUses {
				if other.RunID == use.RunID && other.Step == use.Step && other.Ordinal == use.Ordinal {
					return store.ErrToolUseIdentityConflict
				}
			}
		}
		now := time.Now().UTC()
		if use.ProposedAt.IsZero() {
			use.ProposedAt = now
		}
		if use.CreatedAt.IsZero() {
			use.CreatedAt = now
		}
		if use.UpdatedAt.IsZero() {
			use.UpdatedAt = now
		}
		cp := copyToolUse(&use)
		if err := s.appendToolUseAggregateLocked(cp); err != nil {
			return err
		}
		s.toolUses[use.ID] = &cp
		return nil
	}
	if !sameToolUseIdentityMemory(*existing, use) {
		return store.ErrToolUseIdentityConflict
	}
	use.CreatedAt = existing.CreatedAt
	use.ProposedAt = existing.ProposedAt
	if use.AuthorizedAt.IsZero() {
		use.AuthorizedAt = existing.AuthorizedAt
	}
	if use.StartedAt.IsZero() {
		use.StartedAt = existing.StartedAt
	}
	if use.CompletedAt.IsZero() {
		use.CompletedAt = existing.CompletedAt
	}
	if use.UpdatedAt.IsZero() {
		use.UpdatedAt = existing.UpdatedAt
	}
	if use.Status == existing.Status {
		if !sameToolUseMemory(*existing, use) {
			return store.ErrToolUseInvalidTransition
		}
		return nil
	}
	if !legalToolUseTransitionMemory(existing.Status, use.Status) {
		return store.ErrToolUseInvalidTransition
	}
	if use.Status != store.ToolUseStatusAuthorized && !bytes.Equal(existing.InputJSON, use.InputJSON) {
		return store.ErrToolUseInvalidTransition
	}
	now := time.Now().UTC()
	if use.UpdatedAt.Equal(existing.UpdatedAt) {
		use.UpdatedAt = now
	}
	if use.Status == store.ToolUseStatusAuthorized && use.AuthorizedAt.IsZero() {
		use.AuthorizedAt = now
	}
	if use.Status == store.ToolUseStatusStarted && use.StartedAt.IsZero() {
		use.StartedAt = now
	}
	if terminalToolUseStatus(use.Status) && use.CompletedAt.IsZero() {
		use.CompletedAt = now
	}
	cp := copyToolUse(&use)
	if err := s.appendToolUseAggregateLocked(cp); err != nil {
		return err
	}
	s.toolUses[use.ID] = &cp
	return nil
}

func (s *Store) appendToolUseAggregateLocked(use store.ToolUse) error {
	session, ok := s.sessions[use.SessionID]
	if !ok {
		return store.ErrSessionNotFound
	}
	event, err := store.NewSessionToolUseSavedEvent(use)
	if err != nil {
		return err
	}
	events := s.aggregates[event.Aggregate]
	event.Version = session.AggregateVersion + 1
	projected, err := store.ProjectSession(append(append([]store.AggregateEvent(nil), events...), event))
	if err != nil {
		return err
	}
	s.globalSeq++
	event.GlobalSequence = s.globalSeq
	s.aggregates[event.Aggregate] = append(events, copyAggregateEvent(event))
	s.sessions[use.SessionID] = copySession(projected)
	return nil
}

func (s *Store) ListToolUses(ctx context.Context, sessionID string) ([]store.ToolUse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, store.ErrSessionNotFound
	}
	out := []store.ToolUse{}
	for _, use := range s.toolUses {
		if use.SessionID == sessionID {
			out = append(out, copyToolUse(use))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ProposedAt.Equal(out[j].ProposedAt) {
			return out[i].ProposedAt.Before(out[j].ProposedAt)
		}
		if out[i].Step != out[j].Step {
			return out[i].Step < out[j].Step
		}
		if out[i].Ordinal != out[j].Ordinal {
			return out[i].Ordinal < out[j].Ordinal
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) InterruptActiveToolUses(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, use := range s.toolUses {
		if use.Status == store.ToolUseStatusProposed || use.Status == store.ToolUseStatusAuthorized || use.Status == store.ToolUseStatusStarted {
			updated := copyToolUse(use)
			updated.Status = store.ToolUseStatusInterrupted
			updated.ErrorType = "process_interrupted"
			updated.ErrorMessage = "tool use interrupted because the process stopped"
			updated.CompletedAt = now
			updated.UpdatedAt = now
			if err := s.appendToolUseAggregateLocked(updated); err != nil {
				return err
			}
			*use = updated
		}
	}
	return nil
}

func sameToolUseIdentityMemory(a, b store.ToolUse) bool {
	return a.SessionID == b.SessionID && a.RunID == b.RunID && a.ModelCallID == b.ModelCallID && a.AssistantMessageID == b.AssistantMessageID && a.PartID == b.PartID && a.Step == b.Step && a.Ordinal == b.Ordinal && a.CallID == b.CallID && a.Name == b.Name
}

func sameToolUseMemory(a, b store.ToolUse) bool {
	return sameToolUseIdentityMemory(a, b) && a.Status == b.Status && bytes.Equal(a.InputJSON, b.InputJSON) && a.Output == b.Output && bytes.Equal(a.StructuredJSON, b.StructuredJSON) && bytes.Equal(a.MetadataJSON, b.MetadataJSON) && a.ErrorType == b.ErrorType && a.ErrorMessage == b.ErrorMessage && a.ProposedAt.Equal(b.ProposedAt) && a.AuthorizedAt.Equal(b.AuthorizedAt) && a.StartedAt.Equal(b.StartedAt) && a.CompletedAt.Equal(b.CompletedAt) && a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

func legalToolUseTransitionMemory(from, to string) bool {
	switch from {
	case store.ToolUseStatusProposed:
		return to == store.ToolUseStatusAuthorized || to == store.ToolUseStatusDeclined || to == store.ToolUseStatusFailed || to == store.ToolUseStatusInterrupted
	case store.ToolUseStatusAuthorized:
		return to == store.ToolUseStatusStarted || to == store.ToolUseStatusFailed || to == store.ToolUseStatusInterrupted
	case store.ToolUseStatusStarted:
		return to == store.ToolUseStatusCompleted || to == store.ToolUseStatusFailed || to == store.ToolUseStatusInterrupted
	}
	return false
}

func terminalToolUseStatus(status string) bool {
	return status == store.ToolUseStatusCompleted || status == store.ToolUseStatusFailed || status == store.ToolUseStatusInterrupted || status == store.ToolUseStatusDeclined
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
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
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

func (s *Store) SessionEventWatermark(ctx context.Context, sessionID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return 0, store.ErrSessionNotFound
	}
	var watermark int64
	for _, event := range s.events {
		if event.SessionID == sessionID && event.Seq > watermark {
			watermark = event.Seq
		}
	}
	return watermark, nil
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
