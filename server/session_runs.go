package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

const (
	defaultRunReconcileInterval = 5 * time.Second
	claimRetryDelay             = 100 * time.Millisecond
	claimRetryLimit             = 3
	settleRetryMaxDelay         = 5 * time.Second
)

// sessionRunManager owns daemon-local workers. The queue itself is durable;
// workers only claim and drain already admitted runs.
type sessionRunManager struct {
	server            *Server
	mu                sync.Mutex
	active            map[string]context.CancelFunc
	runCancel         map[string]context.CancelFunc
	pending           map[string]bool
	done              map[string]chan struct{}
	stopped           bool
	reconcileStarted  bool
	reconcileInterval time.Duration
	wg                sync.WaitGroup
}

func newSessionRunManager(server *Server) *sessionRunManager {
	m := &sessionRunManager{
		server:            server,
		active:            map[string]context.CancelFunc{},
		runCancel:         map[string]context.CancelFunc{},
		pending:           map[string]bool{},
		done:              map[string]chan struct{}{},
		reconcileInterval: defaultRunReconcileInterval,
	}
	return m
}

func (m *sessionRunManager) startReconciler() {
	m.mu.Lock()
	if m.stopped || m.reconcileStarted {
		m.mu.Unlock()
		return
	}
	m.reconcileStarted = true
	m.wg.Add(1)
	m.mu.Unlock()
	go m.reconcile()
}

func (m *sessionRunManager) wake(sessionID string) {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
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

func (m *sessionRunManager) reconcile() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.server.ShutdownCtx().Done():
			return
		case <-ticker.C:
			if err := m.resumeQueued(m.server.ShutdownCtx()); err != nil && !errors.Is(err, context.Canceled) {
				m.server.logger.Error("reconcile queued session runs", "error", err)
			}
		}
	}
}

func (m *sessionRunManager) drain(sessionID string, ctx context.Context) {
	defer m.wg.Done()
	for {
		transition, err := m.claim(ctx, sessionID)
		if err != nil {
			m.server.logger.Error("claim session run", "session_id", sessionID, "error", err)
			m.finish(sessionID)
			return
		}
		if transition.Changed {
			m.server.events.publish(transition.Event)
		}
		if transition.Run.ID == "" {
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
		m.execute(ctx, &transition.Run)
		if ctx.Err() != nil {
			m.finish(sessionID)
			return
		}
	}
}

func (m *sessionRunManager) claim(ctx context.Context, sessionID string) (store.SessionRunTransition, error) {
	var lastErr error
	for attempt := 0; attempt < claimRetryLimit; attempt++ {
		transition, err := m.server.store.ClaimNextSessionRun(ctx, sessionID)
		if err == nil {
			return transition, nil
		}
		if errors.Is(err, store.ErrSessionNotFound) || ctx.Err() != nil {
			return store.SessionRunTransition{}, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return store.SessionRunTransition{}, ctx.Err()
		case <-time.After(claimRetryDelay * time.Duration(attempt+1)):
		}
	}
	return store.SessionRunTransition{}, lastErr
}

func (m *sessionRunManager) finish(sessionID string) {
	m.mu.Lock()
	m.finishLocked(sessionID)
	m.mu.Unlock()
}

func (m *sessionRunManager) finishLocked(sessionID string) {
	done, ok := m.done[sessionID]
	if !ok {
		return
	}
	delete(m.active, sessionID)
	delete(m.runCancel, sessionID)
	delete(m.pending, sessionID)
	delete(m.done, sessionID)
	close(done)
}

func (m *sessionRunManager) activeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

func (m *sessionRunManager) execute(workerCtx context.Context, queued *store.SessionRun) {
	m.server.logger.Info("session run started", "session_id", queued.SessionID, "run_id", queued.ID, "agent_id", queued.Agent.ID)
	persistCtx := context.Background()
	runCtx, cancel := context.WithCancel(workerCtx)
	m.mu.Lock()
	m.runCancel[queued.SessionID] = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.runCancel, queued.SessionID)
		m.mu.Unlock()
	}()

	sess := &store.Session{ID: queued.SessionID, WorkDir: queued.WorkDir, WorkspaceID: queued.WorkspaceID, ClientID: queued.ClientID}
	var err error
	var schema *api.OutputSchema
	if len(queued.OutputSchemaJSON) > 0 {
		schema = &api.OutputSchema{}
		err = json.Unmarshal(queued.OutputSchemaJSON, schema)
	}
	if err == nil {
		var runSession *session.Session
		runtimeAgent := queued.Agent
		runtimeAgent.Instructions = queued.EffectiveInstructions
		runSession, err = m.server.buildSessionForRunWithSkills(runCtx, &runtimeAgent, sess, queued.ID, queued.Skills)
		if runSession != nil {
			defer func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanupCancel()
				if cleanupErr := runSession.Close(cleanupCtx); cleanupErr != nil {
					m.server.logger.Error("close run session", "session_id", queued.SessionID, "run_id", queued.ID, "error", cleanupErr)
				}
			}()
		}
		if err == nil && schema != nil {
			runSession.SetOutputSchema(&models.OutputSchema{Name: schema.Name, Schema: schema.Schema})
		}
		if err == nil {
			if queued.Kind == store.SessionRunKindAction {
				err = runSession.RunAction(runCtx, queued.Action, queued.InputJSON, run.SinkFunc(func(event run.Event) {
					m.server.forwardRunEvent(persistCtx, queued.SessionID, queued.ID, session.StreamEvent{Data: event})
				}))
				if err == nil {
					if m.settle(workerCtx, store.SessionRunSettlement{ID: queued.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: store.SessionRunStatusCompleted, EventData: map[string]any{}}) {
						m.server.logger.Info("session action completed", "session_id", queued.SessionID, "run_id", queued.ID, "action", queued.Action)
					}
					return
				}
			} else {
				stream, streamErr := runSession.RunStream(runCtx, queued.Message)
				if streamErr != nil {
					err = streamErr
				} else {
					for stream.Next() {
						m.server.forwardRunEvent(persistCtx, queued.SessionID, queued.ID, stream.Event())
					}
					err = stream.Err()
					if err == nil {
						if runCtx.Err() != nil {
							err = runCtx.Err()
						} else {
							result := stream.Result()
							if m.settle(workerCtx, store.SessionRunSettlement{ID: queued.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: store.SessionRunStatusCompleted, EventData: map[string]any{"usage": result.Usage, "steps": result.Steps}}) {
								m.server.logger.Info("session run completed", "session_id", queued.SessionID, "run_id", queued.ID, "agent_id", queued.Agent.ID, "steps", result.Steps)
							}
							return
						}
					}
				}
			}
		}
	}
	status, errorType := store.SessionRunStatusFailed, "run_failed"
	if errors.Is(runCtx.Err(), context.Canceled) {
		status, errorType = store.SessionRunStatusAborted, "cancelled"
	}
	message := "run failed"
	if err != nil {
		message = err.Error()
	}
	if m.settle(workerCtx, store.SessionRunSettlement{ID: queued.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: status, ErrorType: errorType, ErrorMessage: message, EventData: map[string]any{"error_type": errorType, "error_message": message}}) {
		m.server.logger.Error("session run failed", "session_id", queued.SessionID, "run_id", queued.ID, "agent_id", queued.Agent.ID, "error_type", errorType, "error", message)
	}
}

// settle retries a terminal transition until it commits or the daemon stops.
// A completed provider or tool call cannot be safely repeated, so the worker
// stays pinned to this run rather than advancing the session queue.
func (m *sessionRunManager) settle(ctx context.Context, settlement store.SessionRunSettlement) bool {
	delay := claimRetryDelay
	for attempt := 1; ; attempt++ {
		transition, err := m.server.store.SettleSessionRun(ctx, settlement)
		if err == nil {
			if transition.Changed {
				m.server.events.publish(transition.Event)
			}
			return true
		}
		if ctx.Err() != nil {
			m.server.logger.Error("terminal run settlement deferred to startup recovery", "run_id", settlement.ID, "attempt", attempt, "error", err)
			return false
		}
		m.server.logger.Error("retry terminal run settlement", "run_id", settlement.ID, "attempt", attempt, "retry_in_ms", delay.Milliseconds(), "error", err)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		if delay < settleRetryMaxDelay {
			delay *= 2
			if delay > settleRetryMaxDelay {
				delay = settleRetryMaxDelay
			}
		}
	}
}

func (m *sessionRunManager) abort(sessionID string) int {
	m.mu.Lock()
	cancel, ok := m.runCancel[sessionID]
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

func (m *sessionRunManager) resumeQueued(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sessions, err := m.server.store.ListQueuedSessionRunSessions(ctx)
	if err != nil {
		return err
	}
	for _, sessionID := range sessions {
		m.wake(sessionID)
	}
	return nil
}

func (m *sessionRunManager) stop() {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
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
