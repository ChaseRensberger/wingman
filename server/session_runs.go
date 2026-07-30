package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

// sessionRunManager owns daemon-local workers. The queue itself is durable;
// workers only claim and drain already admitted runs.
type sessionRunManager struct {
	server  *Server
	mu      sync.Mutex
	active  map[string]context.CancelFunc
	pending map[string]bool
	done    map[string]chan struct{}
	wg      sync.WaitGroup
}

func newSessionRunManager(server *Server) *sessionRunManager {
	return &sessionRunManager{
		server:  server,
		active:  map[string]context.CancelFunc{},
		pending: map[string]bool{},
		done:    map[string]chan struct{}{},
	}
}

func (m *sessionRunManager) wake(sessionID string) {
	m.mu.Lock()
	if _, ok := m.active[sessionID]; ok {
		m.pending[sessionID] = true
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.server.ShutdownCtx())
	m.active[sessionID] = cancel
	m.done[sessionID] = make(chan struct{})
	m.wg.Add(1)
	m.mu.Unlock()
	go m.drain(sessionID, ctx)
}

func (m *sessionRunManager) drain(sessionID string, ctx context.Context) {
	defer m.wg.Done()
	for {
		run, err := m.server.store.ClaimNextSessionRun(context.Background(), sessionID)
		if err != nil {
			m.server.logger.Error("claim session run", "session_id", sessionID, "error", err)
			m.finish(sessionID)
			return
		}
		if run == nil {
			m.mu.Lock()
			if m.pending[sessionID] {
				delete(m.pending, sessionID)
				m.mu.Unlock()
				continue
			}
			m.finishLocked(sessionID)
			m.mu.Unlock()
			return
		}
		m.execute(ctx, run)
		if ctx.Err() != nil {
			m.finish(sessionID)
			return
		}
	}
}

func (m *sessionRunManager) finish(sessionID string) {
	m.mu.Lock()
	m.finishLocked(sessionID)
	m.mu.Unlock()
}

func (m *sessionRunManager) finishLocked(sessionID string) {
	done := m.done[sessionID]
	delete(m.active, sessionID)
	delete(m.pending, sessionID)
	delete(m.done, sessionID)
	close(done)
}

func (m *sessionRunManager) execute(ctx context.Context, queued *store.SessionRun) {
	persistCtx := context.Background()
	m.server.persistRunEvent(persistCtx, queued.SessionID, "session.run.started", map[string]any{"run_id": queued.ID})
	sess, err := m.server.store.GetSession(queued.SessionID)
	if err == nil {
		var schema *messageOutputSchema
		if len(queued.OutputSchemaJSON) > 0 {
			schema = &messageOutputSchema{}
			err = json.Unmarshal(queued.OutputSchemaJSON, schema)
		}
		if err == nil {
			var runSession *session.Session
			runSession, err = m.server.buildSession(&queued.Agent, sess)
			if err == nil && schema != nil {
				runSession.SetOutputSchema(&models.OutputSchema{Name: schema.Name, Schema: schema.Schema})
			}
			if err == nil {
				stream, streamErr := runSession.RunStream(ctx, queued.Message)
				if streamErr != nil {
					err = streamErr
				} else {
					for stream.Next() {
						m.server.forwardRunEvent(persistCtx, queued.SessionID, queued.ID, stream.Event())
					}
					err = stream.Err()
					if err == nil {
						result := stream.Result()
						m.server.persistRunEvent(persistCtx, queued.SessionID, "session.run.completed", map[string]any{"run_id": queued.ID, "usage": result.Usage, "steps": result.Steps})
						_ = m.server.store.CompleteSessionRun(persistCtx, queued.ID, store.SessionRunStatusCompleted, "")
						return
					}
				}
			}
		}
	}
	status := store.SessionRunStatusFailed
	if errors.Is(ctx.Err(), context.Canceled) {
		status = store.SessionRunStatusAborted
	}
	message := "run failed"
	if err != nil {
		message = err.Error()
	}
	m.server.persistRunEvent(persistCtx, queued.SessionID, "session.run.failed", map[string]any{"run_id": queued.ID, "error": message, "aborted": status == store.SessionRunStatusAborted})
	_ = m.server.store.CompleteSessionRun(persistCtx, queued.ID, status, message)
}

func (m *sessionRunManager) abort(sessionID string) int {
	m.mu.Lock()
	cancel, ok := m.active[sessionID]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	cancel()
	return 1
}

func (m *sessionRunManager) stopAndWait(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	cancel := m.active[sessionID]
	done := m.done[sessionID]
	delete(m.pending, sessionID)
	m.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *sessionRunManager) resumeQueued(ctx context.Context) {
	sessions, err := m.server.store.ListQueuedSessionRunSessions(ctx)
	if err != nil {
		m.server.logger.Error("list queued session runs", "error", err)
		return
	}
	for _, sessionID := range sessions {
		m.wake(sessionID)
	}
}

func (m *sessionRunManager) wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
