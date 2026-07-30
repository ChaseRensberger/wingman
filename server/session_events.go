package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/go-chi/chi/v5"
)

const defaultSessionEventLimit = 100

type sessionEventBroker struct {
	mu   sync.RWMutex
	subs map[string]map[chan store.SessionEvent]struct{}
}

func newSessionEventBroker() *sessionEventBroker {
	return &sessionEventBroker{subs: make(map[string]map[chan store.SessionEvent]struct{})}
}

func (b *sessionEventBroker) subscribe(sessionID string) (chan store.SessionEvent, func()) {
	ch := make(chan store.SessionEvent, 256)
	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = make(map[chan store.SessionEvent]struct{})
	}
	b.subs[sessionID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[sessionID][ch]; !ok {
			b.mu.Unlock()
			return
		}
		delete(b.subs[sessionID], ch)
		if len(b.subs[sessionID]) == 0 {
			delete(b.subs, sessionID)
		}
		b.mu.Unlock()
		close(ch)
	}
}

func (b *sessionEventBroker) closeSession(sessionID string) {
	b.mu.Lock()
	for ch := range b.subs[sessionID] {
		close(ch)
	}
	delete(b.subs, sessionID)
	b.mu.Unlock()
}

func (b *sessionEventBroker) publish(event store.SessionEvent) {
	b.mu.RLock()
	subs := b.subs[event.SessionID]
	for ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
	b.mu.RUnlock()
}

func (s *Server) appendSessionEvent(ctx context.Context, event store.SessionEvent) (store.SessionEvent, error) {
	stored, err := s.store.AppendSessionEvent(ctx, event)
	if err != nil {
		return store.SessionEvent{}, err
	}
	s.events.publish(stored)
	return stored, nil
}

func (s *Server) publishLiveSessionEvent(event store.SessionEvent) {
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
	event.Data = json.RawMessage(event.DataJSON)
	s.events.publish(event)
}

func newSessionEvent(sessionID, typ string, data any) (store.SessionEvent, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return store.SessionEvent{}, err
	}
	return store.SessionEvent{
		ID:        store.NewID(store.PrefixEvent),
		Type:      typ,
		Time:      time.Now().UTC(),
		SessionID: sessionID,
		DataJSON:  b,
		Data:      json.RawMessage(b),
	}, nil
}

func writeSSEEvent(w http.ResponseWriter, event store.SessionEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	id := event.ID
	if event.Seq > 0 {
		id = strconv.FormatInt(event.Seq, 10)
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, event.Type, b)
	return err
}

func (s *Server) handleSessionEventsHistory(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	after, limit := parseEventQuery(r)
	events, err := s.store.ListSessionEvents(r.Context(), sessionID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": events, "has_more": len(events) == limit})
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	done := s.trackInflight()
	defer done()
	go func() {
		select {
		case <-s.ShutdownCtx().Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	after, limit := parseEventQuery(r)
	live, unsubscribe := s.events.subscribe(sessionID)
	defer unsubscribe()

	stored, err := s.store.ListSessionEvents(ctx, sessionID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastSeq := after
	for _, ev := range stored {
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		if err := writeSSEEvent(w, ev); err != nil {
			return
		}
		flusher.Flush()
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case ev, ok := <-live:
			if !ok {
				return
			}
			if ev.Seq > 0 && ev.Seq <= lastSeq {
				continue
			}
			if ev.Seq > lastSeq {
				lastSeq = ev.Seq
			}
			if err := writeSSEEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func parseEventQuery(r *http.Request) (int64, int) {
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, _ = strconv.ParseInt(raw, 10, 64)
	}
	limit := defaultSessionEventLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	return after, limit
}

func (s *Server) authorizeSessionForRequest(w http.ResponseWriter, r *http.Request, sessionID string) (*store.Session, bool) {
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return nil, false
	}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if sess.ClientID != clientID {
		writeError(w, http.StatusForbidden, "session belongs to another client")
		return nil, false
	}
	return sess, true
}

func (s *Server) forwardRunEvent(ctx context.Context, sessionID, runID string, e session.StreamEvent) {
	data := map[string]any{"run_id": runID}
	switch v := e.Data.(type) {
	case run.IterationStartEvent:
		data["step"] = v.Step
		s.persistRunEvent(ctx, sessionID, "session.step.started", data)
	case run.IterationEndEvent:
		data["step"] = v.Step
		data["usage"] = v.Turn.Usage
		s.persistRunEvent(ctx, sessionID, "session.step.completed", data)
	case run.MessageEvent:
		for _, part := range v.Message.Content {
			switch p := part.(type) {
			case models.TextPart:
				if p.Text != "" {
					s.persistRunEvent(ctx, sessionID, "session.text.completed", map[string]any{"run_id": runID, "text": p.Text})
				}
			case models.ReasoningPart:
				if p.Reasoning != "" {
					s.persistRunEvent(ctx, sessionID, "session.reasoning.completed", map[string]any{"run_id": runID, "text": p.Reasoning})
				}
			}
		}
		data["message"] = v.Message
		s.persistRunEvent(ctx, sessionID, "session.message.created", data)
	case run.ToolExecutionStartEvent:
		data["call_id"] = v.Call.ID
		data["tool"] = v.Call.Name
		data["input"] = v.Call.Args
		s.persistRunEvent(ctx, sessionID, "session.tool.called", data)
		s.persistRunEvent(ctx, sessionID, "session.tool.updated", map[string]any{
			"run_id":     runID,
			"call_id":    v.Call.ID,
			"tool":       v.Call.Name,
			"status":     "running",
			"input":      v.Call.Args,
			"started_at": time.Now().UTC(),
		})
	case run.ToolExecutionProgressEvent:
		s.publishRunEvent(sessionID, "session.tool.progress", map[string]any{
			"run_id":       runID,
			"call_id":      v.CallID,
			"tool":         v.Name,
			"output_delta": v.OutputDelta,
			"metadata":     v.Metadata,
		})
	case run.ToolExecutionEndEvent:
		data["call_id"] = v.Result.CallID
		data["tool"] = v.Result.Name
		data["output"] = v.Result.Output
		data["error"] = v.Result.Error
		data["metadata"] = v.Result.Metadata
		if v.Result.IsError {
			s.persistRunEvent(ctx, sessionID, "session.tool.failed", data)
		} else {
			s.persistRunEvent(ctx, sessionID, "session.tool.completed", data)
		}
		status := "completed"
		if v.Result.IsError {
			status = "error"
		}
		s.persistRunEvent(ctx, sessionID, "session.tool.updated", map[string]any{
			"run_id":       runID,
			"call_id":      v.Result.CallID,
			"tool":         v.Result.Name,
			"status":       status,
			"input":        v.Result.Args,
			"output":       v.Result.Output,
			"metadata":     v.Result.Metadata,
			"error":        v.Result.Error,
			"completed_at": time.Now().UTC(),
			"duration_ms":  v.Result.Duration.Milliseconds(),
		})
	case run.StreamPartEvent:
		s.forwardStreamPart(sessionID, runID, v)
	case run.StructuredOutputEvent:
		data["schema"] = v.Schema
		data["raw_json"] = v.RawJSON
		data["parsed"] = v.Parsed
		s.persistRunEvent(ctx, sessionID, "session.structured_output.completed", data)
	}
}

func (s *Server) forwardStreamPart(sessionID, runID string, e run.StreamPartEvent) {
	base := map[string]any{"run_id": runID, "step": e.Step}
	switch p := e.Part.(type) {
	case models.ToolInputStartPart:
		base["call_id"] = p.ID
		base["tool"] = p.ToolName
		base["status"] = "pending"
		s.publishRunEvent(sessionID, "session.tool.updated", base)
	case models.TextDeltaPart:
		base["delta"] = p.Delta
		s.publishRunEvent(sessionID, "session.text.delta", base)
	case models.ReasoningDeltaPart:
		base["delta"] = p.Delta
		s.publishRunEvent(sessionID, "session.reasoning.delta", base)
	case models.ToolInputDeltaPart:
		base["call_id"] = p.ID
		base["delta"] = p.Delta
		s.publishRunEvent(sessionID, "session.tool.input.delta", base)
	}
}

func (s *Server) persistRunEvent(ctx context.Context, sessionID, typ string, data any) {
	event, err := newSessionEvent(sessionID, typ, data)
	if err != nil {
		s.logger.Error("build session event", "type", typ, "error", err)
		return
	}
	if _, err := s.appendSessionEvent(ctx, event); err != nil && !errors.Is(err, store.ErrSessionNotFound) {
		s.logger.Error("append session event", "type", typ, "session_id", sessionID, "error", err)
	}
}

func (s *Server) publishRunEvent(sessionID, typ string, data any) {
	event, err := newSessionEvent(sessionID, typ, data)
	if err != nil {
		s.logger.Error("build live session event", "type", typ, "error", err)
		return
	}
	s.publishLiveSessionEvent(event)
}
