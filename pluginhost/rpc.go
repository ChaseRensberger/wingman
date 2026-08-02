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
	"time"
)

const (
	// ProtocolVersion is the current Wingman plugin protocol version.
	ProtocolVersion = 1

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
// Protocol v1 activates tools; additional domains require owned daemon
// generations before they can be added here.
type PluginContributions struct {
	Tools []ToolSpec `json:"tools,omitempty"`
}

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

	diagnostics      []diagnosticEntry
	diagnosticsBytes int

	closeMu   sync.Mutex
	closeDone chan struct{}
	closeErr  error
}

type pendingCall struct {
	response chan rpcResponse
	progress func(ToolProgressParams)
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

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

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
	pending := &pendingCall{response: make(chan rpcResponse, 1), progress: progress}
	c.pending[id] = pending
	c.mu.Unlock()

	if err := c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.failPending(id, pending)
		c.terminate(fmt.Errorf("send plugin request: %w", err))
		return fmt.Errorf("send plugin request: %w", err)
	}

	select {
	case <-ctx.Done():
		if c.removePending(id, pending) && c.hasCapability(CapabilityCancellation) {
			_ = c.notify(CancelRequestMethod, map[string]int64{"id": id})
		}
		return ctx.Err()
	case response := <-pending.response:
		return decodeResponse(response, out)
	case <-c.done:
		c.removePending(id, pending)
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

func decodeResponse(response rpcResponse, out any) error {
	if response.Error != nil {
		return fmt.Errorf("plugin error: %s", response.Error.Message)
	}
	if out == nil || len(response.Result) == 0 || string(response.Result) == "null" {
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
	dec := json.NewDecoder(bufio.NewReader(stdout))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if c.isClosing() {
				return
			}
			c.terminate(fmt.Errorf("read plugin message: %w", err))
			return
		}
		c.handleMessage(raw)
	}
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
			c.respondMethodNotFound(envelope.ID, envelope.Method)
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
			pending.progress(progress)
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
	if initialized && !terminated {
		_ = c.requestShutdown()
	}
	_ = c.stdin.Close()
	select {
	case <-c.done:
		return c.Err()
	case <-ctx.Done():
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.done
		return ctx.Err()
	}
}

func (c *rpcClient) requestShutdown() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return c.err
	}
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	return c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: ShutdownMethod})
}

// close preserves the manager's existing no-context call site.
func (c *rpcClient) close() error { return c.Close(context.Background()) }
