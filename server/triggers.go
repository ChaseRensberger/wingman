package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/trigger"
)

type triggerManager struct {
	server  *Server
	store   store.TriggerStore
	mu      sync.Mutex
	started bool
	stopped bool
	done    chan struct{}
}

func newTriggerManager(s *Server) *triggerManager {
	storage, ok := s.store.(store.TriggerStore)
	if !ok {
		return nil
	}
	return &triggerManager{server: s, store: storage, done: make(chan struct{})}
}

func (m *triggerManager) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopped {
		return
	}
	m.started = true
	startedAt := time.Now().UTC()
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := m.tick(m.server.ShutdownCtx(), time.Now().UTC(), startedAt); err != nil && !errors.Is(err, context.Canceled) {
				m.server.logger.Error("check trigger schedules", "error", err)
			}
			select {
			case <-m.server.ShutdownCtx().Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (m *triggerManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started && !m.stopped {
		close(m.done)
	}
	m.stopped = true
}

func (m *triggerManager) wait(ctx context.Context) error {
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *triggerManager) tick(ctx context.Context, now, startedAt time.Time) error {
	items, err := m.store.DueTriggers(ctx, now)
	if err != nil {
		return err
	}
	for _, t := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		next, err := t.Source.Next(now)
		if err != nil {
			return err
		}
		o := trigger.Occurrence{Source: "cron", RequestID: t.NextFireAt.Format(time.RFC3339), ScheduledAt: *t.NextFireAt}
		if o.ScheduledAt.Before(startedAt) || now.Sub(o.ScheduledAt) >= time.Minute {
			o.Status, o.Reason = "skipped", "Missed scheduled times were skipped. Scheduling resumes at the next future time."
		}
		if _, err := m.fire(ctx, t, o, next); err != nil && !errors.Is(err, trigger.ErrConflict) && !errors.Is(err, trigger.ErrNotFound) {
			m.server.logger.Error("submit trigger occurrence", "trigger_id", t.ID, "error", err)
		}
	}
	return nil
}

func (m *triggerManager) fire(ctx context.Context, t trigger.Trigger, o trigger.Occurrence, next time.Time) (trigger.Occurrence, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(m.server.ShutdownCtx(), cancel)
	defer stop()
	if m.server.ShutdownCtx().Err() != nil {
		return trigger.Occurrence{}, context.Canceled
	}
	if existing, err := m.store.GetTriggerOccurrence(ctx, t.ID, o.Source, o.RequestID); err != nil {
		return trigger.Occurrence{}, err
	} else if existing != nil {
		return *existing, nil
	}
	submission := store.TriggerSubmission{Trigger: t, Occurrence: o, NextFireAt: next}
	if o.Status == "" {
		sess, run, err := m.server.prepareTriggerRun(ctx, t)
		if err != nil {
			if ctx.Err() != nil {
				return trigger.Occurrence{}, ctx.Err()
			}
			submission.Occurrence.Status, submission.Occurrence.Reason = "failed", err.Error()
		} else {
			submission.Session, submission.Run = sess, run
		}
	}
	result, err := m.store.CommitTriggerOccurrence(ctx, submission)
	if err != nil {
		return trigger.Occurrence{}, err
	}
	if result.Admission.Created {
		m.server.events.publish(result.Admission.QueuedEvent)
		m.server.runs.wake(result.Admission.Run.SessionID)
	}
	return result.Occurrence, nil
}

func (s *Server) prepareTriggerRun(ctx context.Context, t trigger.Trigger) (store.Session, store.SessionRun, error) {
	var sess store.Session
	var candidate store.SessionRun
	stored, err := s.store.GetAgent(t.Target.AgentID)
	if err != nil {
		return sess, candidate, fmt.Errorf("agent not found: %s", t.Target.AgentID)
	}
	agent := s.agentWithRequestModel(stored, t.Target.ModelRef, nil)
	workDir, workspaceID, err := s.resolveSessionLocation(t.ClientID, t.Target.WorkingDirectory, t.Target.WorkspaceID)
	if err != nil {
		return sess, candidate, err
	}
	sess = store.Session{ID: store.NewID(store.PrefixSession), Title: t.Name, ClientID: t.ClientID, WorkDir: workDir, WorkspaceID: workspaceID}
	instructions, sources, err := s.resolveInstructions(agent, sess.WorkDir)
	if err != nil {
		return sess, candidate, err
	}
	skills, catalog, err := s.resolveSkills(agent, sess.WorkDir)
	if err != nil {
		return sess, candidate, err
	}
	if catalog != "" {
		instructions += "\n\n" + catalog
	}
	runtimeAgent := *agent
	runtimeAgent.Instructions = instructions
	validation, err := s.buildSessionWithSkills(ctx, &runtimeAgent, &sess, skills)
	if err != nil {
		return sess, candidate, err
	}
	closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := validation.Close(closeCtx); err != nil {
		return sess, candidate, fmt.Errorf("close trigger validation session: %w", err)
	}
	candidate = store.SessionRun{SessionID: sess.ID, Message: t.Prompt, Agent: *agent, EffectiveInstructions: instructions, InstructionSources: sources, Skills: skills}
	return sess, candidate, nil
}
