package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chaserensberger/wingman/internal/pluginrpc"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

const (
	// ProtocolVersion is the current Wingman plugin protocol version.
	ProtocolVersion = protocol.Version

	InitializeMethod = "plugin.initialize"
	ShutdownMethod   = "plugin.shutdown"
	HealthMethod     = "plugin.health"

	CapabilityCancellation = "cancellation"
	CapabilityProgress     = "progress"
	CapabilityHealth       = "health"

	ContributionTools = "tools"

	ToolProgressMethod  = "tool.progress"
	PluginLogMethod     = "plugin.log"
	CancelRequestMethod = "$/cancelRequest"

	diagnosticsMaxLines = 200
	diagnosticsMaxBytes = 64 << 10
)

// InitializeParams describes the host and plugin manifest data sent during
// protocol initialization. Plugin is intentionally generic so transport code
// does not depend on the manifest package shape.
type InitializeParams struct {
	HostName                   string         `json:"host_name"`
	HostVersion                string         `json:"host_version"`
	SupportedProtocolVersions  []int          `json:"supported_protocol_versions"`
	SupportedContributionKinds []string       `json:"supported_contribution_kinds"`
	Plugin                     map[string]any `json:"plugin"`
}

// InitializeResult is returned by a plugin after initialization.
type InitializeResult struct {
	ProtocolVersion int                 `json:"protocol_version"`
	Plugin          InitializedPlugin   `json:"plugin"`
	Capabilities    []string            `json:"capabilities,omitempty"`
	Contributions   PluginContributions `json:"contributions,omitempty"`
}

// InitializedPlugin is the identity asserted by the running executable.
type InitializedPlugin struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// PluginContributions contains the contribution domains negotiated at startup.
type PluginContributions = protocol.Contributions

// HealthResult describes plugin health reported by plugin.health.
type HealthResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Details any    `json:"details,omitempty"`
}

// ShutdownResult describes a plugin's graceful shutdown response.
type ShutdownResult struct {
	Message string `json:"message,omitempty"`
}

// ToolProgressParams is sent by a plugin to report progress for a request.
type ToolProgressParams struct {
	RequestID   int64          `json:"request_id"`
	OutputDelta string         `json:"output_delta,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PluginLogParams is sent by a plugin to add a diagnostic record.
type PluginLogParams struct {
	Level   string         `json:"level,omitempty"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// Diagnostic is a bounded plugin stderr or plugin.log record.
type Diagnostic struct {
	Source  string         `json:"source"`
	Level   string         `json:"level,omitempty"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type rpcClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	enc       *json.Encoder
	startedAt time.Time

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]*pendingCall
	err     error
	closed  bool
	closing bool
	done    chan struct{}

	capabilities map[string]bool
	initialized  bool
	processCtx   context.Context
	bindings     map[string]context.Context
	hostHandler  func(context.Context, string, json.RawMessage) (any, error)
	hostCalls    map[string]context.CancelFunc

	diagnostics      []diagnosticEntry
	diagnosticsBytes int

	closeMu   sync.Mutex
	closeDone chan struct{}
	closeErr  error
}

type pendingCall struct {
	callbacksMu   sync.Mutex
	callbacks     sync.WaitGroup
	closing       bool
	emitMu        sync.Mutex
	response      chan rpcResponse
	failure       chan error
	progress      chan progressUpdate
	progressBytes atomic.Int64
	ctx           context.Context
	stream        func(json.RawMessage) error
}

type progressUpdate struct {
	params ToolProgressParams
	bytes  int64
}

func (p *pendingCall) beginCallback() bool {
	p.callbacksMu.Lock()
	defer p.callbacksMu.Unlock()
	if p.closing {
		return false
	}
	p.callbacks.Add(1)
	return true
}

func (p *pendingCall) finishCallbacks() {
	p.callbacksMu.Lock()
	p.closing = true
	p.callbacksMu.Unlock()
	p.callbacks.Wait()
}

type diagnosticEntry struct {
	diagnostic Diagnostic
	bytes      int
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError = pluginrpc.Error

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func startRPC(ctx context.Context, command []string) (*rpcClient, error) {
	if len(command) == 0 || command[0] == "" {
		return nil, errors.New("plugin command is required")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	c := &rpcClient{
		processCtx:   ctx,
		bindings:     map[string]context.Context{},
		hostCalls:    map[string]context.CancelFunc{},
		cmd:          cmd,
		stdin:        stdin,
		enc:          json.NewEncoder(stdin),
		startedAt:    time.Now(),
		pending:      make(map[int64]*pendingCall),
		done:         make(chan struct{}),
		capabilities: make(map[string]bool),
	}
	go c.readStdout(stdout)
	go c.readStderr(stderr)
	go c.wait()
	return c, nil
}

// Done is closed when the plugin process or protocol transport terminates.
func (c *rpcClient) Done() <-chan struct{} { return c.done }

// Err returns the terminal transport error, if any.
func (c *rpcClient) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// PID returns the plugin process ID, or zero before it starts.
func (c *rpcClient) PID() int {
	if c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// StartedAt returns when the plugin process was started.
func (c *rpcClient) StartedAt() time.Time { return c.startedAt }

// Diagnostics returns a snapshot of bounded stderr and plugin.log diagnostics.
func (c *rpcClient) Diagnostics() []Diagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Diagnostic, len(c.diagnostics))
	for i, entry := range c.diagnostics {
		out[i] = entry.diagnostic
		if entry.diagnostic.Fields != nil {
			out[i].Fields = make(map[string]any, len(entry.diagnostic.Fields))
			for k, v := range entry.diagnostic.Fields {
				out[i].Fields[k] = v
			}
		}
	}
	return out
}

// SetCapabilities records capabilities negotiated during initialization.
func (c *rpcClient) SetCapabilities(capabilities []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capabilities = make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		c.capabilities[capability] = true
	}
	c.initialized = true
}

func (c *rpcClient) call(ctx context.Context, method string, params any, out any) error {
	return c.callWithProgress(ctx, method, params, out, nil)
}

func (c *rpcClient) callWithProgress(ctx context.Context, method string, params any, out any, progress func(ToolProgressParams)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.mu.Lock()
	if c.closed {
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = errors.New("plugin transport is closed")
		}
		return err
	}
	c.nextID++
	id := c.nextID
	pending := &pendingCall{response: make(chan rpcResponse, 1), failure: make(chan error, 1), ctx: ctx}
	if progress != nil {
		// The caller drains progress so observers can make RPCs while a progress callback blocks.
		pending.progress = make(chan progressUpdate, 256)
	}
	if sink, ok := ctx.Value(streamKey{}).(func(json.RawMessage) error); ok {
		pending.stream = sink
	}
	c.pending[id] = pending
	c.mu.Unlock()
	defer func() { cancel(); pending.finishCallbacks() }()

	if err := c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.failPending(id, pending)
		c.terminate(fmt.Errorf("send plugin request: %w", err))
		return fmt.Errorf("send plugin request: %w", err)
	}

	deliverProgress := func(update progressUpdate) {
		pending.progressBytes.Add(-update.bytes)
		progress(update.params)
	}
	drainProgress := func() {
		for len(pending.progress) > 0 && ctx.Err() == nil {
			deliverProgress(<-pending.progress)
		}
	}
	for {
		select {
		case update := <-pending.progress:
			deliverProgress(update)
		case <-ctx.Done():
			if c.removePending(id, pending) && c.hasCapability(CapabilityCancellation) {
				_ = c.notify(CancelRequestMethod, map[string]int64{"id": id})
			}
			return ctx.Err()
		case response := <-pending.response:
			drainProgress()
			return decodeResponse(response, out)
		case err := <-pending.failure:
			if c.hasCapability(CapabilityCancellation) {
				_ = c.notify(CancelRequestMethod, map[string]int64{"id": id})
			}
			drainProgress()
			return err
		case <-c.done:
			c.removePending(id, pending)
			drainProgress()
			select {
			case response := <-pending.response:
				return decodeResponse(response, out)
			default:
			}
			if err := c.Err(); err != nil {
				return err
			}
			return errors.New("plugin transport is closed")
		}
	}
}

func decodeResponse(response rpcResponse, out any) error {
	if response.Error != nil {
		return fmt.Errorf("plugin error: %w", response.Error)
	}
	if out == nil || len(response.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Result, out); err != nil {
		return fmt.Errorf("decode plugin result: %w", err)
	}
	return nil
}

func (c *rpcClient) write(message any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.enc.Encode(message)
}

func (c *rpcClient) notify(method string, params any) error {
	return c.write(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *rpcClient) hasCapability(capability string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capabilities[capability]
}

func (c *rpcClient) removePending(id int64, pending *pendingCall) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[id] != pending {
		return false
	}
	delete(c.pending, id)
	return true
}

func (c *rpcClient) failPending(id int64, pending *pendingCall) {
	c.mu.Lock()
	if c.pending[id] == pending {
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func (c *rpcClient) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		c.handleMessage(scanner.Bytes())
	}
	if c.isClosing() {
		return
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.terminate(fmt.Errorf("read plugin message: %w", err))
}

func (c *rpcClient) handleMessage(raw json.RawMessage) {
	var envelope rpcEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		c.terminate(fmt.Errorf("decode plugin message: %w", err))
		return
	}
	if envelope.JSONRPC != "2.0" {
		c.terminate(fmt.Errorf("plugin message has unsupported jsonrpc version %q", envelope.JSONRPC))
		return
	}
	if envelope.Method != "" {
		if len(envelope.ID) != 0 && string(envelope.ID) != "null" {
			if envelope.Method == "host.call" {
				c.dispatchHostCall(envelope)
			} else {
				c.respondMethodNotFound(envelope.ID, envelope.Method)
			}
			return
		}
		c.handleNotification(envelope.Method, envelope.Params)
		return
	}
	if len(envelope.ID) == 0 {
		c.terminate(errors.New("plugin sent JSON-RPC message without method or id"))
		return
	}
	var id int64
	if err := json.Unmarshal(envelope.ID, &id); err != nil {
		c.terminate(fmt.Errorf("plugin response id: %w", err))
		return
	}
	c.mu.Lock()
	pending := c.pending[id]
	if pending != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if pending != nil {
		pending.response <- rpcResponse{JSONRPC: "2.0", ID: id, Result: envelope.Result, Error: envelope.Error}
	}
}

func (c *rpcClient) handleNotification(method string, params json.RawMessage) {
	switch method {
	case CancelRequestMethod:
		var input struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(params, &input) != nil {
			return
		}
		c.mu.Lock()
		cancel := c.hostCalls[string(input.ID)]
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	case "provider.stream":
		var update protocol.StreamUpdate
		if json.Unmarshal(params, &update) != nil {
			return
		}
		c.mu.Lock()
		pending := c.pending[update.RequestID]
		c.mu.Unlock()
		if pending != nil && pending.stream != nil {
			if err := pending.stream(update.Part); err != nil {
				if c.removePending(update.RequestID, pending) {
					pending.failure <- err
				}
			}
		}
	case ToolProgressMethod:
		var progress ToolProgressParams
		if err := json.Unmarshal(params, &progress); err != nil {
			c.appendDiagnostic(Diagnostic{Source: "plugin.log", Level: "warn", Message: "invalid tool.progress notification"})
			return
		}
		c.mu.Lock()
		pending := c.pending[progress.RequestID]
		c.mu.Unlock()
		if pending != nil && pending.progress != nil {
			size := int64(len(params))
			if pending.progressBytes.Add(size) <= 8<<20 {
				select {
				case pending.progress <- progressUpdate{params: progress, bytes: size}:
					return
				default:
				}
			}
			pending.progressBytes.Add(-size)
			if c.removePending(progress.RequestID, pending) {
				pending.failure <- errors.New("plugin tool progress consumer is too slow")
			}
		}
	case PluginLogMethod:
		var log PluginLogParams
		if err := json.Unmarshal(params, &log); err != nil {
			c.appendDiagnostic(Diagnostic{Source: "plugin.log", Level: "warn", Message: string(params)})
			return
		}
		c.appendDiagnostic(Diagnostic{Source: "plugin.log", Level: log.Level, Message: log.Message, Fields: log.Fields})
	}
}

func (c *rpcClient) respondMethodNotFound(id json.RawMessage, method string) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.enc.Encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   rpcError        `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcError{Code: -32601, Message: "method not found: " + method},
	})
}

func (c *rpcClient) readStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 4096), diagnosticsMaxBytes)
	for scanner.Scan() {
		c.appendDiagnostic(Diagnostic{Source: "stderr", Message: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		c.appendDiagnostic(Diagnostic{Source: "stderr", Level: "warn", Message: "read stderr: " + err.Error()})
	}
}

func (c *rpcClient) appendDiagnostic(diagnostic Diagnostic) {
	size := len(diagnostic.Source) + len(diagnostic.Level) + len(diagnostic.Message)
	if diagnostic.Fields != nil {
		if fields, err := json.Marshal(diagnostic.Fields); err == nil {
			size += len(fields)
		}
	}
	if size > diagnosticsMaxBytes {
		diagnostic.Fields = nil
		maxMessageBytes := diagnosticsMaxBytes - len(diagnostic.Source) - len(diagnostic.Level)
		if len(diagnostic.Message) > maxMessageBytes {
			diagnostic.Message = diagnostic.Message[:maxMessageBytes]
		}
		size = len(diagnostic.Source) + len(diagnostic.Level) + len(diagnostic.Message)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.diagnostics) > 0 && (len(c.diagnostics) >= diagnosticsMaxLines || c.diagnosticsBytes+size > diagnosticsMaxBytes) {
		c.diagnosticsBytes -= c.diagnostics[0].bytes
		c.diagnostics = c.diagnostics[1:]
	}
	c.diagnostics = append(c.diagnostics, diagnosticEntry{diagnostic: diagnostic, bytes: size})
	c.diagnosticsBytes += size
}

func (c *rpcClient) wait() {
	err := c.cmd.Wait()
	if err != nil {
		terminal := fmt.Errorf("plugin exited: %w", err)
		c.appendDiagnostic(Diagnostic{Source: "process", Level: "error", Message: terminal.Error()})
		c.terminate(terminal)
		return
	}
	c.terminate(nil)
}

func (c *rpcClient) terminate(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.err = err
	c.pending = make(map[int64]*pendingCall)
	for _, cancel := range c.hostCalls {
		cancel()
	}
	close(c.done)
	c.mu.Unlock()
}

func (c *rpcClient) isClosing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closing
}

// Close gracefully shuts down the plugin. Concurrent callers wait for the
// same shutdown attempt. The process is killed only when ctx expires.
func (c *rpcClient) Close(ctx context.Context) error {
	c.closeMu.Lock()
	if c.closeDone != nil {
		done := c.closeDone
		c.closeMu.Unlock()
		select {
		case <-done:
			c.closeMu.Lock()
			err := c.closeErr
			c.closeMu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.closeDone = make(chan struct{})
	done := c.closeDone
	c.closeMu.Unlock()

	err := c.closeWithContext(ctx)
	c.closeMu.Lock()
	c.closeErr = err
	close(done)
	c.closeMu.Unlock()
	return err
}

func (c *rpcClient) closeWithContext(ctx context.Context) error {
	c.mu.Lock()
	initialized := c.initialized
	terminated := c.closed
	c.closing = true
	c.mu.Unlock()
	var shutdownErr error
	if initialized && !terminated {
		shutdownErr = c.call(ctx, ShutdownMethod, nil, nil)
	}
	_ = c.stdin.Close()
	select {
	case <-c.done:
		return errors.Join(shutdownErr, c.Err())
	case <-ctx.Done():
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.done
		return errors.Join(shutdownErr, ctx.Err())
	}
}

// close preserves the manager's existing no-context call site.
func (c *rpcClient) close() error { return c.Close(context.Background()) }
