package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

// HTTPHooks intercept native model HTTP with lazy, one-shot request and response bodies.
type HTTPHooks struct {
	Request  func(context.Context, models.ModelRef, *http.Request) (*http.Request, error)
	Response func(context.Context, models.ModelRef, *http.Response) (*http.Response, error)
}

type bodySource struct {
	host    *Host
	ctx     context.Context
	pending []byte
	eof     bool
	touched bool
	read    int64
	cancel  context.CancelFunc
	cleanup context.Context
	once    sync.Once
}

func (b *bodySource) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.pending) == 0 && !b.eof {
		b.touched = true
		var chunk protocol.BodyChunk
		if err := b.host.Call(b.ctx, "http.body.read", nil, &chunk); err != nil {
			return 0, err
		}
		b.pending, b.eof = chunk.Data, chunk.EOF
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	b.read += int64(n)
	if n == 0 && b.eof {
		return 0, io.EOF
	}
	return n, nil
}
func (b *bodySource) Close() error {
	b.once.Do(func() {
		b.cancel()
		ctx, cancel := context.WithTimeout(b.cleanup, 5*time.Second)
		defer cancel()
		_ = b.host.Call(ctx, "scope.release", nil, nil)
	})
	return nil
}

type pluginBody struct {
	mu     sync.Mutex
	body   io.ReadCloser
	source *bodySource
	once   sync.Once
	err    error
}

func (b *pluginBody) close() error {
	b.once.Do(func() { b.source.Close(); b.err = b.body.Close() })
	return b.err
}

type httpBodies struct {
	mu     sync.Mutex
	next   uint64
	bodies map[string]*pluginBody
}

func (b *httpBodies) close() {
	b.mu.Lock()
	values := b.bodies
	b.bodies = nil
	b.mu.Unlock()
	for _, value := range values {
		_ = value.close()
	}
}

func (b *httpBodies) call(ctx context.Context, e *Extension, method string, raw json.RawMessage) (any, error) {
	if method == "http.body.read" || method == "http.body.close" {
		var input protocol.BodyRead
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		b.mu.Lock()
		value := b.bodies[input.ID]
		if method == "http.body.close" {
			delete(b.bodies, input.ID)
		}
		b.mu.Unlock()
		if value == nil {
			if method == "http.body.close" {
				return nil, nil
			}
			return nil, errors.New("HTTP body is closed")
		}
		if method == "http.body.close" {
			return nil, value.close()
		}
		value.mu.Lock()
		defer value.mu.Unlock()
		stop := context.AfterFunc(ctx, func() { _ = value.close() })
		defer stop()
		data := make([]byte, 32*1024)
		n, err := value.body.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return protocol.BodyChunk{Data: data[:n], EOF: errors.Is(err, io.EOF)}, nil
	}
	var input protocol.HTTPExchange
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	var token string
	if err := e.host.Call(ctx, "http.body.bind", nil, &token); err != nil {
		return nil, err
	}
	cleanup := context.WithValue(e.host.ctx, scopeKey{}, token)
	streamCtx, cancel := context.WithCancel(cleanup)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	source := &bodySource{host: e.host, ctx: streamCtx, cleanup: cleanup, cancel: cancel, eof: !input.HasBody}
	keep := false
	defer func() {
		if !keep {
			_ = source.Close()
		}
	}()
	var body io.ReadCloser = source
	if method == "http.request" {
		request, err := http.NewRequestWithContext(streamCtx, input.Method, input.URL, source)
		if err != nil {
			return nil, err
		}
		request.Header, request.ContentLength = input.Headers, input.ContentLength
		if e.HTTP.Request == nil {
			return nil, errors.New("HTTP request hook is not registered")
		}
		request, err = e.HTTP.Request(ctx, input.Model, request)
		if err != nil {
			if request != nil && request.Body != nil {
				_ = request.Body.Close()
			}
			return nil, err
		}
		if request == nil || request.URL == nil {
			return nil, errors.New("HTTP request hook returned no request")
		}
		input.Method, input.URL, input.Headers, input.ContentLength = request.Method, request.URL.String(), request.Header, request.ContentLength
		body = request.Body
	} else {
		if e.HTTP.Response == nil {
			return nil, errors.New("HTTP response hook is not registered")
		}
		request, err := http.NewRequestWithContext(streamCtx, input.Method, input.URL, nil)
		if err != nil {
			return nil, err
		}
		response, err := e.HTTP.Response(ctx, input.Model, &http.Response{Request: request, Status: fmt.Sprintf("%d %s", input.Status, http.StatusText(input.Status)), StatusCode: input.Status, Header: input.Headers, ContentLength: input.ContentLength, Body: source})
		if err != nil {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			return nil, err
		}
		if response == nil {
			return nil, errors.New("HTTP response hook returned no response")
		}
		input.Status, input.Headers, input.ContentLength = response.StatusCode, response.Header, response.ContentLength
		body = response.Body
	}
	input.HasBody = body != nil
	if err := ctx.Err(); err != nil {
		if body != nil {
			_ = body.Close()
		}
		return nil, err
	}
	if body == source && input.ContentLength >= 0 {
		input.ContentLength -= source.read
	}
	if body != nil && (body != source || source.touched) {
		b.mu.Lock()
		if b.bodies == nil {
			b.bodies = map[string]*pluginBody{}
		}
		b.next++
		input.BodyID = fmt.Sprint(b.next)
		b.bodies[input.BodyID] = &pluginBody{body: body, source: source}
		keep = true
		b.mu.Unlock()
	}
	return input, nil
}
