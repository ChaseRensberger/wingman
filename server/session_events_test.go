package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type streamRecorder struct {
	header     http.Header
	mu         sync.Mutex
	body       bytes.Buffer
	flushes    int
	blockFlush int
	blocked    chan struct{}
	release    chan struct{}
	blockOnce  sync.Once
}

func newStreamRecorder(blockFlush int) *streamRecorder {
	return &streamRecorder{header: make(http.Header), blockFlush: blockFlush, blocked: make(chan struct{}), release: make(chan struct{})}
}

func (w *streamRecorder) Header() http.Header { return w.header }
func (w *streamRecorder) WriteHeader(int)     {}
func (w *streamRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}
func (w *streamRecorder) Flush() {
	w.mu.Lock()
	w.flushes++
	block := w.flushes == w.blockFlush
	w.mu.Unlock()
	if block {
		w.blockOnce.Do(func() { close(w.blocked) })
		<-w.release
	}
}
func (w *streamRecorder) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func testEventServer(t *testing.T) (*Server, *memory.Store, *store.Session) {
	t.Helper()
	data := memory.NewStore()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	session := &store.Session{ID: "ses_events", ClientID: client.ID}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	return New(Config{Store: data}), data, session
}

func streamRequest(ctx context.Context, sessionID, query string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/events"+query, nil).WithContext(ctx)
	return request
}

func awaitEvent(t *testing.T, recorder *streamRecorder, typ string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(recorder.String(), "event: "+typ+"\n") {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("did not receive %s:\n%s", typ, recorder.String())
}

type recordedSessionEvent struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	Cursor        *struct {
		Seq int64 `json:"seq"`
	} `json:"cursor"`
}

type gapSessionEventStore struct{ store.Store }

func (s *gapSessionEventStore) SessionEventWatermark(context.Context, string) (int64, error) {
	return 2, nil
}

func (s *gapSessionEventStore) ListSessionEvents(context.Context, string, int64, int) ([]store.SessionEvent, error) {
	return []store.SessionEvent{{ID: "evt_gap", SessionID: "ses_events", Seq: 2, Type: "stored", DataJSON: []byte(`{}`)}}, nil
}

func recordedEvents(t *testing.T, recorder *streamRecorder) []recordedSessionEvent {
	t.Helper()
	var events []recordedSessionEvent
	for _, frame := range strings.Split(recorder.String(), "\n\n") {
		lines := strings.Split(frame, "\n")
		var event recordedSessionEvent
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
					t.Fatal(err)
				}
			}
		}
		if event.Type != "" {
			events = append(events, event)
		}
	}
	return events
}

func TestSessionEventsReplaysAllPagesBeforeSynchronized(t *testing.T) {
	server, data, session := testEventServer(t)
	for i := 0; i < 1001; i++ {
		if _, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "stored"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	recorder := newStreamRecorder(0)
	done := make(chan struct{})
	go func() { server.router.ServeHTTP(recorder, streamRequest(ctx, session.ID, "?limit=500")); close(done) }()
	awaitEvent(t, recorder, "session.events.synchronized")
	cancel()
	<-done
	events := recordedEvents(t, recorder)
	if len(events) != 1002 || events[1001].Type != "session.events.synchronized" || events[1001].Cursor == nil || events[1001].Cursor.Seq != 1001 {
		t.Fatalf("events boundary = len %d last %#v", len(events), events[len(events)-1])
	}
	for i, event := range events[:1001] {
		if event.SchemaVersion != 1 || event.Cursor == nil || event.Cursor.Seq != int64(i+1) {
			t.Fatalf("event %d cursor = %#v", i, event.Cursor)
		}
	}
	if events[1001].SchemaVersion != 1 {
		t.Fatalf("synchronized schema version = %d", events[1001].SchemaVersion)
	}
}

func TestSessionEventsBuffersEventsCommittedDuringReplay(t *testing.T) {
	server, data, session := testEventServer(t)
	if _, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "first"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := newStreamRecorder(1)
	done := make(chan struct{})
	go func() { server.router.ServeHTTP(recorder, streamRequest(ctx, session.ID, "")); close(done) }()
	<-recorder.blocked
	if _, err := server.appendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	close(recorder.release)
	awaitEvent(t, recorder, "concurrent")
	cancel()
	<-done
	events := recordedEvents(t, recorder)
	if len(events) != 3 || events[0].Cursor == nil || events[0].Cursor.Seq != 1 || events[1].Type != "session.events.synchronized" || events[1].Cursor == nil || events[1].Cursor.Seq != 1 || events[2].Cursor == nil || events[2].Cursor.Seq != 2 {
		t.Fatalf("events = %#v", events)
	}
}

func TestSessionEventsBackfillsOutOfOrderDurablePublication(t *testing.T) {
	server, data, session := testEventServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := newStreamRecorder(0)
	done := make(chan struct{})
	go func() { server.router.ServeHTTP(recorder, streamRequest(ctx, session.ID, "")); close(done) }()
	awaitEvent(t, recorder, "session.events.synchronized")
	first, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "second"})
	if err != nil {
		t.Fatal(err)
	}
	server.events.publish(second)
	awaitEvent(t, recorder, "second")
	cancel()
	<-done
	events := recordedEvents(t, recorder)
	if len(events) < 3 || events[1].Cursor == nil || events[1].Cursor.Seq != first.Seq || events[2].Cursor == nil || events[2].Cursor.Seq != second.Seq {
		t.Fatalf("events = %#v", events)
	}
}

func TestSessionEventsOverflowRequiresResync(t *testing.T) {
	server, _, session := testEventServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := newStreamRecorder(2)
	done := make(chan struct{})
	go func() { server.router.ServeHTTP(recorder, streamRequest(ctx, session.ID, "")); close(done) }()
	awaitEvent(t, recorder, "session.events.synchronized")
	server.publishLiveSessionEvent(store.SessionEvent{SessionID: session.ID, Type: "blocked"})
	<-recorder.blocked
	for i := 0; i < 257; i++ {
		server.publishLiveSessionEvent(store.SessionEvent{SessionID: session.ID, Type: "overflow"})
	}
	close(recorder.release)
	awaitEvent(t, recorder, "session.events.resync_required")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not disconnect after overflow")
	}
}

func TestSessionEventBrokerDiagnosticsTrackBacklogAndClosures(t *testing.T) {
	broker := newSessionEventBroker()
	_, firstUnsubscribe := broker.subscribe("ses_first")
	_, secondUnsubscribe := broker.subscribe("ses_second")
	broker.publish(store.SessionEvent{SessionID: "ses_first"})
	broker.publish(store.SessionEvent{SessionID: "ses_first"})
	broker.publish(store.SessionEvent{SessionID: "ses_second"})

	subscribers, backlog, maxBacklog, overflows, closures := broker.diagnostics()
	if subscribers != 2 || backlog != 3 || maxBacklog != 2 || overflows != 0 || closures != 0 {
		t.Fatalf("diagnostics = subscribers:%d backlog:%d max_backlog:%d overflows:%d closures:%d", subscribers, backlog, maxBacklog, overflows, closures)
	}
	firstUnsubscribe()
	broker.closeSession("ses_second")
	secondUnsubscribe()
	subscribers, backlog, maxBacklog, overflows, closures = broker.diagnostics()
	if subscribers != 0 || backlog != 0 || maxBacklog != 0 || overflows != 0 || closures != 2 {
		t.Fatalf("diagnostics after close = subscribers:%d backlog:%d max_backlog:%d overflows:%d closures:%d", subscribers, backlog, maxBacklog, overflows, closures)
	}
}

func TestSessionEventCursorUsesExplicitAfterBeforeLastEventID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/?after=7", nil)
	request.Header.Set("Last-Event-ID", "3")
	if after, _ := parseEventQuery(request); after != 7 {
		t.Fatalf("after = %d, want 7", after)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Last-Event-ID", "3")
	if after, _ := parseEventQuery(request); after != 3 {
		t.Fatalf("after = %d, want 3", after)
	}
	request = httptest.NewRequest(http.MethodGet, "/?after=-1", nil)
	request.Header.Set("Last-Event-ID", strconv.Itoa(3))
	if after, _ := parseEventQuery(request); after != 0 {
		t.Fatalf("after = %d, want 0", after)
	}
}

func TestSessionEventsRequiresResyncWhenCursorIsAhead(t *testing.T) {
	server, _, session := testEventServer(t)
	recorder := newStreamRecorder(0)
	server.router.ServeHTTP(recorder, streamRequest(context.Background(), session.ID, "?after=7"))

	events := recordedEvents(t, recorder)
	if len(events) != 1 || events[0].Type != "session.events.resync_required" {
		t.Fatalf("events = %#v", events)
	}
}

func TestSessionEventsRequiresResyncForDurableGap(t *testing.T) {
	server, data, session := testEventServer(t)
	server.store = &gapSessionEventStore{Store: data}
	recorder := newStreamRecorder(0)
	server.router.ServeHTTP(recorder, streamRequest(context.Background(), session.ID, ""))

	events := recordedEvents(t, recorder)
	if len(events) != 1 || events[0].Type != "session.events.resync_required" {
		t.Fatalf("events = %#v", events)
	}
}

func TestSessionEventHistoryHasMoreUsesWatermark(t *testing.T) {
	server, data, session := testEventServer(t)
	for i := 0; i < 2; i++ {
		if _, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "stored"}); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, streamRequest(context.Background(), session.ID, "/history?after=0&limit=2"))
	var page struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			SchemaVersion int `json:"schema_version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("exact final page reports has_more")
	}
	if len(page.Data) != 2 || page.Data[0].SchemaVersion != 1 || page.Data[1].SchemaVersion != 1 {
		t.Fatalf("history schema versions = %#v", page.Data)
	}
	if _, err := data.AppendSessionEvent(context.Background(), store.SessionEvent{SessionID: session.ID, Type: "stored"}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, streamRequest(context.Background(), session.ID, "/history?after=0&limit=2"))
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if !page.HasMore {
		t.Fatal("partial page does not report has_more")
	}
}
