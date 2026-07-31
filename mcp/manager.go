package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chaserensberger/wingman/tool"
)

const defaultTimeout = 30 * time.Second
const retirementTimeout = 5 * time.Second

var sanitizeRE = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// Status is the runtime state of one configured MCP server.
type Status struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	Error     string   `json:"error,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	ToolCount int      `json:"tool_count"`
}

// ToolInfo describes one MCP-backed Wingman tool.
type ToolInfo struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	Source       string         `json:"source"`
	Server       string         `json:"server"`
	RemoteName   string         `json:"remote_name"`
	Status       string         `json:"status"`
}

// Manager owns configured MCP client sessions and exposes their tools.
type Manager struct {
	cfg Config

	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	servers     map[string]*serverState
	closed      bool
	connect     connector
}

type serverState struct {
	name       string
	cfg        ServerConfig
	status     string
	err        string
	generation *connectionGeneration
}

type connection interface {
	CallTool(context.Context, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error)
	Close() error
}

type connector func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error)

type connectionGeneration struct {
	connection connection
	tools      []*mcpsdk.Tool

	mu        sync.Mutex
	accepting bool
	calls     sync.WaitGroup
}

// New creates a manager and connects all enabled configured servers.
func New(ctx context.Context, cfg Config) *Manager {
	m := newManager(cfg, connectServer)
	m.ConnectEnabled(ctx)
	return m
}

func newManager(cfg Config, connect connector) *Manager {
	m := &Manager{cfg: cloneConfig(cfg).normalized(), servers: map[string]*serverState{}, connect: connect}
	for name, serverCfg := range m.cfg.Servers {
		m.servers[name] = &serverState{name: name, cfg: serverCfg, status: "disabled"}
	}
	return m
}

// ConnectEnabled connects every enabled configured server.
func (m *Manager) ConnectEnabled(ctx context.Context) {
	names := make([]string, 0, len(m.cfg.Servers))
	for name, cfg := range m.cfg.Servers {
		if cfg.isEnabled() {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		_ = m.Connect(ctx, name)
	}
}

// Close disconnects all active MCP sessions.
func (m *Manager) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), retirementTimeout)
	defer cancel()
	return m.CloseContext(ctx)
}

// CloseContext disconnects all active MCP sessions within one shared deadline.
func (m *Manager) CloseContext(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	retired := make([]*connectionGeneration, 0, len(m.servers))
	for _, state := range m.servers {
		if state.generation != nil {
			retireLocked(state.generation)
			retired = append(retired, state.generation)
			state.generation = nil
		}
		state.status = "disabled"
		state.err = ""
	}
	m.mu.Unlock()

	return retireAll(ctx, retired)
}

// Status returns a stable snapshot of configured MCP server status.
func (m *Manager) Status() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Status, 0, len(names))
	for _, name := range names {
		state := m.servers[name]
		var defs []*mcpsdk.Tool
		if state.generation != nil {
			defs = state.generation.tools
		}
		tools := make([]string, 0, len(defs))
		for _, def := range defs {
			tools = append(tools, toolName(name, def.Name))
		}
		sort.Strings(tools)
		out = append(out, Status{
			Name:      name,
			Type:      state.cfg.Type,
			Status:    state.status,
			Error:     state.err,
			Tools:     tools,
			ToolCount: len(tools),
		})
	}
	return out
}

// Tools returns all connected MCP tools as Wingman runtime tools.
func (m *Manager) Tools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []tool.Tool
	for _, state := range m.servers {
		if state.generation == nil || state.status != "connected" {
			continue
		}
		for _, def := range state.generation.tools {
			out = append(out, &mcpTool{generation: state.generation, server: state.name, remoteName: def.Name, def: def})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name() != out[j].Name() {
			return out[i].Name() < out[j].Name()
		}
		left := out[i].(*mcpTool)
		right := out[j].(*mcpTool)
		if left.server != right.server {
			return left.server < right.server
		}
		return left.remoteName < right.remoteName
	})
	return out
}

// ToolInfos returns catalog metadata for connected MCP tools.
func (m *Manager) ToolInfos() []ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ToolInfo
	for _, state := range m.servers {
		if state.generation == nil || state.status != "connected" {
			continue
		}
		for _, def := range state.generation.tools {
			out = append(out, ToolInfo{
				Name:         toolName(state.name, def.Name),
				Description:  def.Description,
				InputSchema:  schemaMap(def.InputSchema),
				OutputSchema: schemaMap(def.OutputSchema),
				Source:       "mcp",
				Server:       state.name,
				RemoteName:   def.Name,
				Status:       state.status,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Server != out[j].Server {
			return out[i].Server < out[j].Server
		}
		return out[i].RemoteName < out[j].RemoteName
	})
	return out
}

// Connect connects or reconnects a configured MCP server.
func (m *Manager) Connect(ctx context.Context, name string) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.RLock()
	state := m.servers[name]
	closed := m.closed
	m.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("MCP server not found: %s", name)
	}
	if closed {
		return errors.New("MCP manager is closed")
	}
	if !state.cfg.isEnabled() {
		return m.disconnectLocked(name)
	}

	conn, tools, err := m.connect(ctx, name, state.cfg)
	if err == nil && conn == nil {
		err = errors.New("MCP connection is nil")
	}
	if err != nil && conn != nil {
		_ = conn.Close()
	}
	m.mu.Lock()
	state = m.servers[name]
	if state == nil {
		m.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
		return fmt.Errorf("MCP server not found: %s", name)
	}
	if err != nil {
		if state.generation == nil {
			state.status = "failed"
		} else {
			state.status = "connected"
		}
		state.err = err.Error()
		m.mu.Unlock()
		return err
	}

	newGeneration := &connectionGeneration{connection: conn, tools: tools, accepting: true}
	oldGeneration := state.generation
	if oldGeneration != nil {
		retireLocked(oldGeneration)
	}
	state.generation = newGeneration
	state.status = "connected"
	state.err = ""
	m.mu.Unlock()

	return retireWithTimeout(oldGeneration)
}

// Disconnect closes a connected MCP server without removing its config.
func (m *Manager) Disconnect(name string) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.disconnectLocked(name)
}

func (m *Manager) disconnectLocked(name string) error {
	m.mu.Lock()
	state := m.servers[name]
	if state == nil {
		m.mu.Unlock()
		return fmt.Errorf("MCP server not found: %s", name)
	}
	oldGeneration := state.generation
	if oldGeneration != nil {
		retireLocked(oldGeneration)
	}
	state.generation = nil
	state.status = "disabled"
	state.err = ""
	m.mu.Unlock()
	return retireWithTimeout(oldGeneration)
}

func (g *connectionGeneration) callTool(ctx context.Context, remoteName string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	g.mu.Lock()
	if !g.accepting {
		g.mu.Unlock()
		return nil, errors.New("MCP connection has been retired")
	}
	g.calls.Add(1)
	g.mu.Unlock()
	defer g.calls.Done()
	return g.connection.CallTool(ctx, &mcpsdk.CallToolParams{Name: remoteName, Arguments: args})
}

func retireLocked(g *connectionGeneration) {
	g.mu.Lock()
	g.accepting = false
	g.mu.Unlock()
}

func retireWithTimeout(g *connectionGeneration) error {
	ctx, cancel := context.WithTimeout(context.Background(), retirementTimeout)
	defer cancel()
	return retire(ctx, g)
}

func retire(ctx context.Context, g *connectionGeneration) error {
	if g == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		g.calls.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return errors.Join(ctx.Err(), g.connection.Close())
	}
	return g.connection.Close()
}

func retireAll(ctx context.Context, generations []*connectionGeneration) error {
	errs := make(chan error, len(generations))
	for _, generation := range generations {
		go func() { errs <- retire(ctx, generation) }()
	}
	joined := make([]error, 0, len(generations))
	for range generations {
		joined = append(joined, <-errs)
	}
	return errors.Join(joined...)
}

func connectServer(ctx context.Context, name string, cfg ServerConfig) (connection, []*mcpsdk.Tool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout(cfg))
	defer cancel()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "wingman", Version: "dev"}, &mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}})

	transport, err := transportFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil && cfg.Type == "remote" {
		session, err = connectSSE(ctx, cfg)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("connect %s: %w", name, err)
	}
	tools, err := listTools(ctx, session)
	if err != nil {
		_ = session.Close()
		return nil, nil, err
	}
	return session, tools, nil
}

func connectSSE(ctx context.Context, cfg ServerConfig) (*mcpsdk.ClientSession, error) {
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "wingman", Version: "dev"}, &mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}})
	return client.Connect(ctx, &mcpsdk.SSEClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient(cfg.Headers)}, nil)
}

func transportFor(cfg ServerConfig) (mcpsdk.Transport, error) {
	switch cfg.Type {
	case "local":
		if len(cfg.Command) == 0 || cfg.Command[0] == "" {
			return nil, fmt.Errorf("local MCP server command is required")
		}
		cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
		if cfg.CWD != "" {
			cmd.Dir = expandHome(cfg.CWD)
		}
		cmd.Env = os.Environ()
		for key, value := range cfg.Environment {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		return &mcpsdk.CommandTransport{Command: cmd}, nil
	case "remote":
		if cfg.URL == "" {
			return nil, fmt.Errorf("remote MCP server url is required")
		}
		return &mcpsdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient(cfg.Headers)}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP server type %q", cfg.Type)
	}
}

func listTools(ctx context.Context, session *mcpsdk.ClientSession) ([]*mcpsdk.Tool, error) {
	var out []*mcpsdk.Tool
	var cursor string
	seen := map[string]struct{}{}
	for page := 0; page < 1000; page++ {
		res, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		out = append(out, res.Tools...)
		if res.NextCursor == "" {
			return out, nil
		}
		if _, ok := seen[res.NextCursor]; ok {
			return nil, fmt.Errorf("list MCP tools returned duplicate cursor %q", res.NextCursor)
		}
		seen[res.NextCursor] = struct{}{}
		cursor = res.NextCursor
	}
	return nil, fmt.Errorf("list MCP tools exceeded page limit")
}

func timeout(cfg ServerConfig) time.Duration {
	if cfg.Timeout > 0 {
		return time.Duration(cfg.Timeout) * time.Millisecond
	}
	return defaultTimeout
}

func toolName(server, remote string) string {
	return sanitize(server) + "_" + sanitize(remote)
}

func sanitize(value string) string {
	return sanitizeRE.ReplaceAllString(value, "_")
}

func schemaMap(raw any) map[string]any {
	if raw == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil || out == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if out["type"] == nil {
		out["type"] = "object"
	}
	if out["properties"] == nil {
		out["properties"] = map[string]any{}
	}
	return out
}

func httpClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return http.DefaultClient
	}
	return &http.Client{Transport: headerTransport{base: http.DefaultTransport, headers: headers}}
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	return t.base.RoundTrip(clone)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
