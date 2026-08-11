package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/chaserensberger/wingman/api"
)

// SessionEventsOptions configures a persistent-session event stream.
type SessionEventsOptions struct {
	After *int64
	Limit *int
}

// SSEFrame is one parsed server-sent event frame.
type SSEFrame struct {
	ID    string
	Event string
	Data  []byte
}

// ListSessionEvents returns one page of durable session events.
func (c *SDK) ListSessionEvents(ctx context.Context, sessionID string, options *SessionEventsOptions) (api.SessionEventPage, error) {
	query := sessionEventsQuery(options)
	path := "/sessions/" + url.PathEscape(sessionID) + "/events/history"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	endpoint, err := c.baseURL.Parse(path)
	if err != nil {
		return api.SessionEventPage{}, fmt.Errorf("resolve request URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return api.SessionEventPage{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.doer.Do(request)
	if err != nil {
		return api.SessionEventPage{}, fmt.Errorf("request Wingman API: %w", err)
	}
	defer response.Body.Close()
	var page api.SessionEventPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		return api.SessionEventPage{}, fmt.Errorf("decode session event page: %w", err)
	}
	return page, nil
}

// RunStream reads events from one ephemeral run.
type RunStream struct {
	decoder *sseDecoder
	frame   SSEFrame
	event   api.RunStreamEvent
	err     error
}

// Next advances to the next run event.
func (s *RunStream) Next() bool {
	frame, ok := s.decoder.Next()
	if !ok {
		s.err = s.decoder.Err()
		return false
	}
	event, err := decodeRunStreamEvent(frame.Data)
	if err != nil {
		s.err = err
		return false
	}
	s.event = event
	s.frame = frame
	return true
}

// Event returns the most recent event.
func (s *RunStream) Event() api.RunStreamEvent { return s.event }

// Frame returns the SSE frame for the most recent event.
func (s *RunStream) Frame() SSEFrame { return s.frame }

// Err returns the terminal stream error.
func (s *RunStream) Err() error { return s.err }

// Close closes the stream before it reaches a terminal event.
func (s *RunStream) Close() error { return s.decoder.Close() }

// SessionEventStream reads durable and live events for one session.
type SessionEventStream struct {
	decoder *sseDecoder
	frame   SSEFrame
	event   api.SessionEvent
	err     error
}

// Next advances to the next session event.
func (s *SessionEventStream) Next() bool {
	frame, ok := s.decoder.Next()
	if !ok {
		s.err = s.decoder.Err()
		return false
	}
	if err := json.Unmarshal(frame.Data, &s.event); err != nil {
		s.err = fmt.Errorf("decode session event: %w", err)
		return false
	}
	s.frame = frame
	return true
}

// Event returns the most recent event.
func (s *SessionEventStream) Event() api.SessionEvent { return s.event }

// Frame returns the SSE frame for the most recent event.
func (s *SessionEventStream) Frame() SSEFrame { return s.frame }

// Err returns the terminal stream error.
func (s *SessionEventStream) Err() error { return s.err }

// Close closes the stream before the server disconnects it.
func (s *SessionEventStream) Close() error { return s.decoder.Close() }

// Run starts an ephemeral run and returns its event stream.
func (c *SDK) Run(ctx context.Context, body RunRequest) (*RunStream, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode run request: %w", err)
	}
	response, err := c.sseRequest(ctx, http.MethodPost, "/run", bytes.NewReader(encoded), nil)
	if err != nil {
		return nil, err
	}
	return &RunStream{decoder: newSSEDecoder(response.Body, c.maxSSEEventBytes)}, nil
}

// StreamSessionEvents opens a durable session event stream. Reconnect using the
// last received event cursor when the stream ends.
func (c *SDK) StreamSessionEvents(ctx context.Context, sessionID string, options *SessionEventsOptions) (*SessionEventStream, error) {
	query := sessionEventsQuery(options)
	path := "/sessions/" + url.PathEscape(sessionID) + "/events"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	response, err := c.sseRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SessionEventStream{decoder: newSSEDecoder(response.Body, c.maxSSEEventBytes)}, nil
}

func sessionEventsQuery(options *SessionEventsOptions) url.Values {
	query := url.Values{}
	if options == nil {
		return query
	}
	if options.After != nil {
		query.Set("after", strconv.FormatInt(*options.After, 10))
	}
	if options.Limit != nil {
		query.Set("limit", strconv.Itoa(*options.Limit))
	}
	return query
}

func (c *SDK) sseRequest(ctx context.Context, method, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	endpoint, err := c.baseURL.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("resolve request URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		request.Header[name] = append([]string(nil), values...)
	}
	response, err := c.doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Wingman API: %w", err)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		response.Body.Close()
		return nil, fmt.Errorf("expected text/event-stream response, got %q", contentType)
	}
	return response, nil
}

type sseDecoder struct {
	reader      *bufio.Reader
	body        io.Closer
	maxBytes    int
	lastEventID string
	err         error
}

func newSSEDecoder(body io.ReadCloser, maxBytes int) *sseDecoder {
	return &sseDecoder{reader: bufio.NewReader(body), body: body, maxBytes: maxBytes}
}

func (d *sseDecoder) Next() (SSEFrame, bool) {
	var data []string
	event := ""
	size := 0
	for {
		line, err := d.readLine()
		if err != nil && err != io.EOF {
			d.err = fmt.Errorf("read server-sent event: %w", err)
			return SSEFrame{}, false
		}
		size += len(line) + 1
		if size > d.maxBytes {
			d.err = fmt.Errorf("server-sent event exceeds %d bytes", d.maxBytes)
			return SSEFrame{}, false
		}
		if line == "" {
			if len(data) == 0 {
				if err == io.EOF {
					return SSEFrame{}, false
				}
				continue
			}
			return SSEFrame{ID: d.lastEventID, Event: event, Data: []byte(strings.Join(data, "\n"))}, true
		}
		if strings.HasPrefix(line, ":") {
			if err == io.EOF {
				return SSEFrame{}, false
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			if !strings.ContainsRune(value, '\x00') {
				d.lastEventID = value
			}
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
		if err == io.EOF {
			if len(data) == 0 {
				return SSEFrame{}, false
			}
			return SSEFrame{ID: d.lastEventID, Event: event, Data: []byte(strings.Join(data, "\n"))}, true
		}
	}
}

func (d *sseDecoder) readLine() (string, error) {
	var line []byte
	for {
		fragment, err := d.reader.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > d.maxBytes {
			return "", fmt.Errorf("line exceeds %d bytes", d.maxBytes)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		line = bytes.TrimSuffix(line, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		return string(line), err
	}
}

func (d *sseDecoder) Err() error { return d.err }

func (d *sseDecoder) Close() error { return d.body.Close() }

func decodeRunStreamEvent(value []byte) (api.RunStreamEvent, error) {
	var raw struct {
		Type    api.RunStreamEventType `json:"type"`
		Version int                    `json:"version"`
		Data    json.RawMessage        `json:"data"`
	}
	if err := json.Unmarshal(value, &raw); err != nil {
		return api.RunStreamEvent{}, fmt.Errorf("decode run event envelope: %w", err)
	}

	var data api.RunStreamEventData
	switch raw.Type {
	case api.RunStreamEventIterationStart:
		data = &api.RunIterationStartEventData{}
	case api.RunStreamEventIterationEnd:
		data = &api.RunIterationEndEventData{}
	case api.RunStreamEventMessage:
		data = &api.RunMessageEventData{}
	case api.RunStreamEventToolProposed:
		data = &api.RunToolProposedEventData{}
	case api.RunStreamEventToolAuthorized:
		data = &api.RunToolAuthorizedEventData{}
	case api.RunStreamEventToolStart:
		data = &api.RunToolStartEventData{}
	case api.RunStreamEventToolProgress:
		data = &api.RunToolProgressEventData{}
	case api.RunStreamEventToolEnd:
		data = &api.RunToolEndEventData{}
	case api.RunStreamEventStreamPart:
		data = &api.RunStreamPartEventData{}
	case api.RunStreamEventCompaction, api.RunStreamEventContextTransformed:
		data = &api.RunContextTransformedEventData{}
	case api.RunStreamEventError:
		data = &api.RunErrorEventData{}
	case api.RunStreamEventStructuredOutput:
		data = &api.RunStructuredOutputEventData{}
	case api.RunStreamEventDone:
		data = &api.RunDoneEventData{}
	default:
		var unknown any
		if err := json.Unmarshal(raw.Data, &unknown); err != nil {
			return api.RunStreamEvent{}, fmt.Errorf("decode unknown run event data: %w", err)
		}
		return api.RunStreamEvent{Type: raw.Type, Version: raw.Version, Data: api.UnknownRunStreamEventData{Value: unknown}}, nil
	}
	if err := json.Unmarshal(raw.Data, data); err != nil {
		return api.RunStreamEvent{}, fmt.Errorf("decode %s run event data: %w", raw.Type, err)
	}
	return api.RunStreamEvent{Type: raw.Type, Version: raw.Version, Data: data}, nil
}
