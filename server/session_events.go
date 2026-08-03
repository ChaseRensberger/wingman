package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/go-chi/chi/v5"
)

const defaultSessionEventLimit = 100

type sessionEventBroker struct {
	mu        sync.RWMutex
	subs      map[string]map[*sessionEventSubscription]struct{}
	overflows atomic.Int64
	closures  atomic.Int64
}

type sessionEventSubscription struct {
	events   chan store.SessionEvent
	done     chan struct{}
	overflow chan struct{}
	doneOnce sync.Once
	overOnce sync.Once
}

func newSessionEventSubscription() *sessionEventSubscription {
	return &sessionEventSubscription{
		events:   make(chan store.SessionEvent, 256),
		done:     make(chan struct{}),
		overflow: make(chan struct{}),
	}
}

func (s *sessionEventSubscription) close() (closed bool) {
	s.doneOnce.Do(func() {
		close(s.done)
		closed = true
	})
	return closed
}

func (s *sessionEventSubscription) signalOverflow() (signaled bool) {
	s.overOnce.Do(func() {
		close(s.overflow)
		signaled = true
	})
	return signaled
}

func newSessionEventBroker() *sessionEventBroker {
	return &sessionEventBroker{subs: make(map[string]map[*sessionEventSubscription]struct{})}
}

func (b *sessionEventBroker) subscribe(sessionID string) (*sessionEventSubscription, func()) {
	sub := newSessionEventSubscription()
	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = make(map[*sessionEventSubscription]struct{})
	}
	b.subs[sessionID][sub] = struct{}{}
	b.mu.Unlock()
	return sub, func() {
		b.mu.Lock()
		if subs := b.subs[sessionID]; subs != nil {
			if _, ok := subs[sub]; ok {
				delete(subs, sub)
				if len(subs) == 0 {
					delete(b.subs, sessionID)
				}
			}
		}
		b.mu.Unlock()
		if sub.close() {
			b.closures.Add(1)
		}
	}
}

func (b *sessionEventBroker) closeSession(sessionID string) {
	b.mu.Lock()
	for sub := range b.subs[sessionID] {
		if sub.close() {
			b.closures.Add(1)
		}
	}
	delete(b.subs, sessionID)
	b.mu.Unlock()
}

func (b *sessionEventBroker) publish(event store.SessionEvent) {
	b.mu.RLock()
	subs := b.subs[event.SessionID]
	for sub := range subs {
		select {
		case sub.events <- event:
		default:
			if sub.signalOverflow() {
				b.overflows.Add(1)
			}
		}
	}
	b.mu.RUnlock()
}

func (b *sessionEventBroker) diagnostics() (subscribers, backlog, maxBacklog int, overflows, closures int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, subs := range b.subs {
		subscribers += len(subs)
		for sub := range subs {
			queued := len(sub.events)
			backlog += queued
			if queued > maxBacklog {
				maxBacklog = queued
			}
		}
	}
	return subscribers, backlog, maxBacklog, b.overflows.Load(), b.closures.Load()
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
		ID:            store.NewID(store.PrefixEvent),
		SchemaVersion: api.CurrentSessionEventSchemaVersion,
		Type:          typ,
		Time:          time.Now().UTC(),
		SessionID:     sessionID,
		DataJSON:      b,
		Data:          json.RawMessage(b),
	}, nil
}

func writeSSEEvent(w http.ResponseWriter, event store.SessionEvent) error {
	public, err := apiSessionEvent(event)
	if err != nil {
		return err
	}
	b, err := json.Marshal(public)
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
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	watermark, err := s.store.SessionEventWatermark(r.Context(), sessionID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastSeq := after
	if len(events) > 0 {
		lastSeq = events[len(events)-1].Seq
	}
	public, err := apiSessionEvents(events)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.SessionEventPage{Data: public, HasMore: lastSeq < watermark})
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
		s.writeError(w, http.StatusInternalServerError, "streaming not supported")
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

	watermark, err := s.store.SessionEventWatermark(ctx, sessionID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if after > watermark {
		s.writeSessionEventsResync(w, flusher, sessionID, watermark, "resume cursor is ahead of durable history")
		return
	}
	lastSeq := after
	if err := s.replaySessionEvents(ctx, w, flusher, sessionID, &lastSeq, watermark, limit); err != nil {
		s.writeSessionEventsResync(w, flusher, sessionID, lastSeq, "unable to replay durable events")
		return
	}
	if sessionEventSubscriptionOverflow(live) {
		s.writeSessionEventsResync(w, flusher, sessionID, lastSeq, "subscriber overflow during replay")
		return
	}
	// This boundary is live-only; its sequence is the durable cursor a client
	// should use to resume, not a separately stored event.
	synchronized, _ := newSessionEvent(sessionID, string(api.SessionEventEventsSynchronized), api.EventsSynchronizedEventData{Cursor: watermark, Watermark: watermark})
	synchronized.Seq = watermark
	synchronized.ID = strconv.FormatInt(watermark, 10)
	if err := writeSSEEvent(w, synchronized); err != nil {
		return
	}
	flusher.Flush()
	lastSeq = watermark

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		if sessionEventSubscriptionOverflow(live) {
			s.writeSessionEventsResync(w, flusher, sessionID, lastSeq, "subscriber overflow")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-live.done:
			return
		case <-live.overflow:
			s.writeSessionEventsResync(w, flusher, sessionID, lastSeq, "subscriber overflow")
			return
		case ev := <-live.events:
			if ev.Seq == 0 {
				if err := writeSSEEvent(w, ev); err != nil {
					return
				}
				flusher.Flush()
				continue
			}
			if ev.Seq <= lastSeq {
				continue
			}
			if ev.Seq > lastSeq+1 {
				if err := s.replaySessionEvents(ctx, w, flusher, sessionID, &lastSeq, ev.Seq, limit); err != nil {
					s.writeSessionEventsResync(w, flusher, sessionID, lastSeq, "unable to backfill durable events")
					return
				}
				continue
			}
			if err := writeSSEEvent(w, ev); err != nil {
				return
			}
			lastSeq = ev.Seq
			flusher.Flush()
		}
	}
}

func sessionEventSubscriptionOverflow(sub *sessionEventSubscription) bool {
	select {
	case <-sub.overflow:
		return true
	default:
		return false
	}
}

func (s *Server) replaySessionEvents(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, sessionID string, lastSeq *int64, through int64, limit int) error {
	for *lastSeq < through {
		events, err := s.store.ListSessionEvents(ctx, sessionID, *lastSeq, limit)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("session event sequence gap before %d", through)
		}
		for _, event := range events {
			if event.Seq > through {
				return nil
			}
			if event.Seq <= *lastSeq {
				continue
			}
			if event.Seq != *lastSeq+1 {
				return fmt.Errorf("session event sequence gap at %d", *lastSeq+1)
			}
			if err := writeSSEEvent(w, event); err != nil {
				return err
			}
			*lastSeq = event.Seq
			flusher.Flush()
		}
	}
	return nil
}

func (s *Server) writeSessionEventsResync(w http.ResponseWriter, flusher http.Flusher, sessionID string, cursor int64, reason string) {
	event, _ := newSessionEvent(sessionID, string(api.SessionEventEventsResyncRequired), api.EventsResyncRequiredEventData{Cursor: cursor, Reason: reason})
	event.Seq = cursor
	event.ID = strconv.FormatInt(cursor, 10)
	if writeSSEEvent(w, event) == nil {
		flusher.Flush()
	}
}

func parseEventQuery(r *http.Request) (int64, int) {
	var after int64
	query := r.URL.Query()
	raw, explicitAfter := query["after"]
	if explicitAfter && len(raw) > 0 {
		if cursor, err := strconv.ParseInt(raw[0], 10, 64); err == nil && cursor >= 0 {
			after = cursor
		}
	} else if cursor, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && cursor >= 0 {
		after = cursor
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
		s.writeError(w, http.StatusNotFound, err.Error())
		return nil, false
	}
	clientID, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if sess.ClientID != clientID {
		s.writeError(w, http.StatusForbidden, "session belongs to another client")
		return nil, false
	}
	return sess, true
}

func (s *Server) forwardRunEvent(ctx context.Context, sessionID, runID string, e session.StreamEvent) {
	switch v := e.Data.(type) {
	case run.IterationStartEvent:
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventStepStarted), api.StepEventData{RunID: runID, Step: v.Step})
	case run.IterationEndEvent:
		usage := v.Turn.Usage
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventStepCompleted), api.StepEventData{RunID: runID, Step: v.Step, Usage: &usage})
	case run.MessageEvent:
		for _, part := range v.Message.Content {
			switch p := part.(type) {
			case models.TextPart:
				if p.Text != "" {
					s.persistRunEvent(ctx, sessionID, string(api.SessionEventTextCompleted), api.ContentCompletedEventData{RunID: runID, MessageID: v.Message.ID, PartID: p.ID, Revision: v.Message.Revision, Text: p.Text})
				}
			case models.ReasoningPart:
				if p.Reasoning != "" {
					s.persistRunEvent(ctx, sessionID, string(api.SessionEventReasoningCompleted), api.ContentCompletedEventData{RunID: runID, MessageID: v.Message.ID, PartID: p.ID, Revision: v.Message.Revision, Text: p.Reasoning})
				}
			}
		}
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventMessageCreated), api.MessageCreatedEventData{RunID: runID, Message: v.Message})
	case run.ToolUseProposedEvent:
		toolData := toolCallEventData(runID, v.Call)
		toolData.Status = "proposed"
		toolData.ProposedAt = formatEventTime(v.Call.ProposedAt)
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolCalled), toolData)
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolUpdated), toolData)
	case run.ToolUseAuthorizedEvent:
		toolData := toolCallEventData(runID, v.Call)
		toolData.Status = "authorized"
		toolData.AuthorizedAt = formatEventTime(v.Call.AuthorizedAt)
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolUpdated), toolData)
	case run.ToolExecutionStartEvent:
		toolData := toolCallEventData(runID, v.Call)
		toolData.Status = "started"
		toolData.StartedAt = formatEventTime(v.Call.StartedAt)
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolUpdated), toolData)
	case run.ToolExecutionProgressEvent:
		s.publishRunEvent(sessionID, string(api.SessionEventToolProgress), api.ToolEventData{
			RunID: runID, CallID: v.CallID, ToolUseID: v.ToolUseID, Tool: v.Name,
			OutputDelta: v.OutputDelta, Metadata: v.Metadata,
		})
	case run.ToolExecutionEndEvent:
		data := api.ToolEventData{
			RunID: runID, CallID: v.Result.CallID, ToolUseID: v.Result.ToolUseID, Tool: v.Result.Name,
			Status: string(v.Result.Status), Output: v.Result.Output, Structured: v.Result.Structured,
			Error: v.Result.Error, Metadata: v.Result.Metadata,
		}
		if v.Result.IsError {
			s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolFailed), data)
		} else {
			s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolCompleted), data)
		}
		status := string(v.Result.Status)
		if status == "" {
			status = "completed"
			if v.Result.IsError {
				status = "failed"
			}
		}
		data.Status = status
		data.Input = v.Result.Args
		data.CompletedAt = formatEventTime(time.Now().UTC())
		data.DurationMS = v.Result.Duration.Milliseconds()
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventToolUpdated), data)
	case run.StreamPartEvent:
		s.forwardStreamPart(sessionID, runID, v)
	case run.StructuredOutputEvent:
		s.persistRunEvent(ctx, sessionID, string(api.SessionEventStructuredOutputCompleted), api.StructuredOutputEventData{RunID: runID, Schema: v.Schema, RawJSON: v.RawJSON, Parsed: v.Parsed})
	}
}

func toolCallEventData(runID string, call run.ToolCall) api.ToolEventData {
	return api.ToolEventData{
		RunID: runID, ToolUseID: call.ToolUseID, CallID: call.ID, Tool: call.Name, Input: call.Args,
		Step: call.Step, Ordinal: call.Ordinal, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID,
	}
}

func formatEventTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Server) forwardStreamPart(sessionID, runID string, e run.StreamPartEvent) {
	switch p := e.Part.(type) {
	case models.ToolInputStartPart:
		s.publishRunEvent(sessionID, string(api.SessionEventToolUpdated), api.ToolEventData{
			RunID: runID, Step: e.Step, MessageID: e.MessageID, PartID: e.PartID, Revision: e.Revision,
			CallID: p.ID, Tool: p.ToolName, Status: "pending",
		})
	case models.TextDeltaPart:
		s.publishRunEvent(sessionID, string(api.SessionEventTextDelta), api.ContentDeltaEventData{RunID: runID, Step: e.Step, MessageID: e.MessageID, PartID: e.PartID, Revision: e.Revision, Delta: p.Delta})
	case models.ReasoningDeltaPart:
		s.publishRunEvent(sessionID, string(api.SessionEventReasoningDelta), api.ContentDeltaEventData{RunID: runID, Step: e.Step, MessageID: e.MessageID, PartID: e.PartID, Revision: e.Revision, Delta: p.Delta})
	case models.ToolInputDeltaPart:
		s.publishRunEvent(sessionID, string(api.SessionEventToolInputDelta), api.ContentDeltaEventData{RunID: runID, Step: e.Step, MessageID: e.MessageID, PartID: e.PartID, Revision: e.Revision, CallID: p.ID, Delta: p.Delta})
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
