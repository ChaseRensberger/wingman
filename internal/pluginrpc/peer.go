// Package pluginrpc implements concurrent, bidirectional plugin messages.
package pluginrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const maxMessage = 16 << 20

// Handler handles an incoming request without blocking the reader.
type Handler func(context.Context, string, json.RawMessage) (any, error)

// Peer multiplexes calls, cancellation, and notifications on one connection.
type Peer struct {
	writer  io.Writer
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[int64]chan message
	active  map[int64]context.CancelFunc
	next    atomic.Int64
	done    chan struct{}
	closed  bool
	handler Handler
	wg      sync.WaitGroup
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type requestKey struct{}

// RequestID returns the host request associated with a handler context.
func RequestID(ctx context.Context) int64 {
	id, _ := ctx.Value(requestKey{}).(int64)
	return id
}

// New creates a peer. Serve must run while Call is used.
func New(writer io.Writer, handler Handler) *Peer {
	return &Peer{writer: writer, handler: handler, pending: map[int64]chan message{}, active: map[int64]context.CancelFunc{}, done: make(chan struct{})}
}

// Serve reads messages until EOF or a protocol failure.
func (p *Peer) Serve(ctx context.Context, reader io.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer p.Close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxMessage)
	for scanner.Scan() {
		var msg message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return fmt.Errorf("decode plugin message: %w", err)
		}
		if msg.JSONRPC != "2.0" {
			return errors.New("unsupported plugin message version")
		}
		if msg.Method == "" {
			p.mu.Lock()
			ch := p.pending[msg.ID]
			delete(p.pending, msg.ID)
			p.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if msg.Method == "$/cancelRequest" {
			var input struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(msg.Params, &input); err != nil {
				return err
			}
			p.mu.Lock()
			cancel := p.active[input.ID]
			p.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			continue
		}
		callCtx, cancel := context.WithCancel(context.WithValue(ctx, requestKey{}, msg.ID))
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			cancel()
			return io.EOF
		}
		if msg.ID != 0 {
			if _, exists := p.active[msg.ID]; exists {
				p.mu.Unlock()
				cancel()
				return errors.New("duplicate active plugin request")
			}
			p.active[msg.ID] = cancel
		}
		p.wg.Add(1)
		p.mu.Unlock()
		go p.dispatch(callCtx, cancel, msg)
	}
	return scanner.Err()
}

func (p *Peer) dispatch(ctx context.Context, cancel context.CancelFunc, msg message) {
	defer p.wg.Done()
	defer cancel()
	defer func() { p.mu.Lock(); delete(p.active, msg.ID); p.mu.Unlock() }()
	result, err := func() (result any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("plugin handler panic: %v", recovered)
			}
		}()
		if p.handler == nil {
			return nil, errors.New("plugin method is unavailable")
		}
		return p.handler(ctx, msg.Method, msg.Params)
	}()
	if msg.ID == 0 {
		return
	}
	response := message{JSONRPC: "2.0", ID: msg.ID}
	if err == nil {
		response.Result, err = json.Marshal(result)
	}
	if err != nil {
		response.Error = NewError(err)
	}
	_ = p.write(response)
}

// Call waits for one reply and propagates cancellation to the other peer.
func (p *Peer) Call(ctx context.Context, method string, input, output any) error {
	params, err := json.Marshal(input)
	if err != nil {
		return err
	}
	id := p.next.Add(1)
	ch := make(chan message, 1)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return io.ErrClosedPipe
	}
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	if err := p.write(message{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		_ = p.Notify("$/cancelRequest", map[string]int64{"id": id})
		return ctx.Err()
	case <-p.done:
		return io.ErrClosedPipe
	case msg := <-ch:
		if msg.Error != nil {
			return msg.Error
		}
		if output != nil {
			return json.Unmarshal(msg.Result, output)
		}
		return nil
	}
}

// Notify sends a message that does not expect a reply.
func (p *Peer) Notify(method string, input any) error {
	params, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return p.write(message{JSONRPC: "2.0", Method: method, Params: params})
}

func (p *Peer) write(msg message) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return json.NewEncoder(p.writer).Encode(msg)
}

// Close cancels handlers and releases pending callers without replaying work.
func (p *Peer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	for _, cancel := range p.active {
		cancel()
	}
	close(p.done)
}

// Wait waits for handlers after the connection has closed.
func (p *Peer) Wait() { p.wg.Wait() }
