// Package execution owns resources shared by sessions in one canonical execution scope.
package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	wingmcp "github.com/chaserensberger/wingman/mcp"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/tool"
)

const defaultIdleTimeout = time.Minute
const defaultCloseTimeout = 30 * time.Second

// Config defines the immutable inputs used to construct execution scopes.
type Config struct {
	RootContext    context.Context
	PluginDirs     []string
	DisablePlugins bool
	MCP            map[string]wingmcp.ServerConfig
	Providers      *provider.Registry
	NativeTools    []tool.Tool
	IdleTimeout    time.Duration
}

// BuiltinTools returns a fresh deterministic set of Wingman's native tools.
func BuiltinTools() []tool.Tool {
	tools := []tool.Tool{
		tool.NewApplyPatchTool(), tool.NewBashTool(), tool.NewReadTool(),
		tool.NewWriteTool(), tool.NewEditTool(), tool.NewGlobTool(),
		tool.NewGrepTool(), tool.NewWebFetchTool(), tool.NewWebSearchTool(),
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools
}

type factories struct {
	newPlugins func(context.Context, []string) (*pluginhost.Manager, error)
	newMCP     func(context.Context, wingmcp.Config) *wingmcp.Manager
}

// Manager caches and owns canonical execution scopes.
type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config
	f      factories

	mu     sync.Mutex
	scopes map[string]*ownedScope
	closed bool
}

// Count returns the number of cached execution scopes, including the daemon scope.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.scopes)
}

type ownedScope struct {
	scope   *Scope
	ready   chan struct{}
	err     error
	cancel  context.CancelFunc
	waiters int
	refs    int
	timer   *time.Timer
}

// Scope is one immutable resource graph for a directory or the directoryless scope.
type Scope struct {
	id        string
	workDir   string
	providers *provider.Registry
	native    []tool.Tool
	plugins   *pluginhost.Manager
	mcp       *wingmcp.Manager
	cancel    context.CancelFunc

	closeOnce sync.Once
	closeErr  error
}

// Lease pins a scope until Close releases it.
type Lease struct {
	manager *Manager
	owned   *ownedScope
	once    sync.Once
}

// NewManager constructs the directoryless scope and pins it for daemon APIs.
func NewManager(cfg Config) (*Manager, error) {
	return newManager(cfg, factories{
		newPlugins: pluginhost.New,
		newMCP:     wingmcp.New,
	})
}

func newManager(cfg Config, f factories) (*Manager, error) {
	if cfg.Providers == nil {
		return nil, fmt.Errorf("provider registry is required")
	}
	if err := (wingmcp.Config{Servers: cfg.MCP}).Validate(); err != nil {
		return nil, err
	}
	root := cfg.RootContext
	if root == nil {
		root = context.Background()
	}
	ctx, cancel := context.WithCancel(root)
	cfg.PluginDirs = append([]string(nil), cfg.PluginDirs...)
	cfg.NativeTools = append([]tool.Tool(nil), cfg.NativeTools...)
	cfg.MCP = cloneMCP(cfg.MCP)
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}
	m := &Manager{ctx: ctx, cancel: cancel, cfg: cfg, f: f, scopes: map[string]*ownedScope{}}
	scopeCtx, scopeCancel := context.WithCancel(m.ctx)
	scope, err := m.construct(scopeCtx, scopeCancel, "", "")
	if err != nil {
		cancel()
		return nil, err
	}
	m.scopes[""] = &ownedScope{scope: scope, refs: 1}
	return m, nil
}

// Acquire returns a lease for workDir's canonical scope. An empty directory
// selects the daemon's directoryless scope.
func (m *Manager) Acquire(ctx context.Context, workDir string) (*Lease, error) {
	id, canonical, err := canonicalScope(workDir)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("execution scope manager is closed")
	}
	owned := m.scopes[id]
	if owned == nil {
		scopeCtx, scopeCancel := context.WithCancel(m.ctx)
		owned = &ownedScope{ready: make(chan struct{}), cancel: scopeCancel, waiters: 1}
		m.scopes[id] = owned
		go m.constructOwned(owned, scopeCtx, id, canonical)
	} else if owned.ready != nil {
		owned.waiters++
	}
	ready := owned.ready
	m.mu.Unlock()
	if ready != nil {
		select {
		case <-ready:
		case <-ctx.Done():
			m.abandon(owned, id)
			return nil, ctx.Err()
		case <-m.ctx.Done():
			m.abandon(owned, id)
			return nil, fmt.Errorf("execution scope manager is closed")
		}
	}
	if err := ctx.Err(); err != nil {
		m.abandon(owned, id)
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ready != nil {
		owned.waiters--
	}
	if owned.err != nil {
		return nil, owned.err
	}
	if m.closed || m.scopes[id] != owned || owned.scope == nil {
		return nil, fmt.Errorf("execution scope manager is closed")
	}
	if owned.timer != nil {
		owned.timer.Stop()
		owned.timer = nil
	}
	owned.refs++
	return &Lease{manager: m, owned: owned}, nil
}

// Providers returns the immutable provider generation shared by current scopes.
func (m *Manager) Providers() *provider.Registry { return m.cfg.Providers }

func (m *Manager) abandon(owned *ownedScope, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owned.waiters > 0 {
		owned.waiters--
	}
	if owned.waiters == 0 && owned.ready != nil {
		if m.scopes != nil && m.scopes[id] == owned {
			delete(m.scopes, id)
		}
		owned.cancel()
	} else if owned.waiters == 0 && owned.scope != nil && owned.refs == 0 && owned.scope.id != "" && owned.timer == nil {
		owned.timer = time.AfterFunc(m.cfg.IdleTimeout, func() { m.evict(owned) })
	}
}

func (m *Manager) constructOwned(owned *ownedScope, ctx context.Context, id, workDir string) {
	scope, err := m.construct(ctx, owned.cancel, id, workDir)
	m.mu.Lock()
	ready := owned.ready
	if m.closed && err == nil {
		err = fmt.Errorf("execution scope manager is closed")
	}
	if err != nil && scope != nil {
		m.mu.Unlock()
		_ = closeScope(scope)
		m.mu.Lock()
	}
	if err != nil {
		owned.err = err
		if m.scopes != nil && m.scopes[id] == owned {
			delete(m.scopes, id)
		}
	} else {
		owned.scope = scope
		if owned.waiters == 0 {
			owned.timer = time.AfterFunc(m.cfg.IdleTimeout, func() { m.evict(owned) })
		}
	}
	close(ready)
	owned.ready = nil
	m.mu.Unlock()
}

// Close cancels and closes every scope, including scopes with active leases.
func (m *Manager) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()
	return m.CloseContext(ctx)
}

// CloseContext cancels and closes every scope within one shared deadline.
func (m *Manager) CloseContext(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.cancel()
	scopes := make([]*Scope, 0, len(m.scopes))
	var pending []<-chan struct{}
	for _, owned := range m.scopes {
		if owned.timer != nil {
			owned.timer.Stop()
		}
		if owned.scope != nil {
			scopes = append(scopes, owned.scope)
		} else if owned.ready != nil {
			pending = append(pending, owned.ready)
		}
	}
	m.scopes = nil
	m.mu.Unlock()
	var errs []error
	for _, ready := range pending {
		select {
		case <-ready:
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
		}
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].id > scopes[j].id })
	closed := make(chan error, len(scopes))
	for _, scope := range scopes {
		go func() { closed <- scope.close(ctx) }()
	}
	for range scopes {
		errs = append(errs, <-closed)
	}
	return errors.Join(errs...)
}

func (m *Manager) construct(ctx context.Context, cancel context.CancelFunc, id, workDir string) (*Scope, error) {
	s := &Scope{id: id, workDir: workDir, providers: m.cfg.Providers, native: append([]tool.Tool(nil), m.cfg.NativeTools...), cancel: cancel}
	if !m.cfg.DisablePlugins {
		dirs := append([]string(nil), m.cfg.PluginDirs...)
		if local := pluginhost.LocalPluginDir(workDir); local != "" {
			dirs = append(dirs, local)
		}
		plugins, err := m.f.newPlugins(ctx, dirs)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("initialize plugins for scope %q: %w", id, err)
		}
		s.plugins = plugins
	}
	s.mcp = m.f.newMCP(ctx, wingmcp.Config{Servers: cloneMCP(m.cfg.MCP)})
	if _, err := s.ToolCatalog(); err != nil {
		_ = closeScope(s)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = closeScope(s)
		return nil, err
	}
	return s, nil
}

func (m *Manager) release(owned *ownedScope) {
	m.mu.Lock()
	if m.closed || m.scopes[owned.scope.id] != owned {
		m.mu.Unlock()
		return
	}
	owned.refs--
	if owned.refs > 0 || owned.scope.id == "" {
		m.mu.Unlock()
		return
	}
	owned.timer = time.AfterFunc(m.cfg.IdleTimeout, func() { m.evict(owned) })
	m.mu.Unlock()
}

func (m *Manager) evict(owned *ownedScope) {
	m.mu.Lock()
	if m.closed || m.scopes[owned.scope.id] != owned || owned.refs != 0 {
		m.mu.Unlock()
		return
	}
	delete(m.scopes, owned.scope.id)
	m.mu.Unlock()
	_ = closeScope(owned.scope)
}

// Scope returns the pinned execution scope.
func (l *Lease) Scope() *Scope { return l.owned.scope }

// Close releases the scope lease. It is idempotent.
func (l *Lease) Close(context.Context) error {
	l.once.Do(func() { l.manager.release(l.owned) })
	return nil
}

// ID returns the canonical scope identity. The directoryless scope is empty.
func (s *Scope) ID() string { return s.id }

// WorkDir returns the canonical working directory. It is empty for the directoryless scope.
func (s *Scope) WorkDir() string { return s.workDir }

// Providers returns the immutable provider generation for this scope.
func (s *Scope) Providers() *provider.Registry { return s.providers }

// Plugins returns the scope-owned external plugin manager, if enabled.
func (s *Scope) Plugins() *pluginhost.Manager { return s.plugins }

// MCP returns the scope-owned MCP manager.
func (s *Scope) MCP() *wingmcp.Manager { return s.mcp }

// ToolCatalog composes one immutable tool catalog from current owned generations.
func (s *Scope) ToolCatalog() (*tool.Registry, error) {
	tools := append([]tool.Tool(nil), s.native...)
	if s.plugins != nil {
		tools = append(tools, s.plugins.Tools()...)
	}
	if s.mcp != nil {
		tools = append(tools, s.mcp.Tools()...)
	}
	registry, err := tool.Compose(tools)
	if err != nil {
		return nil, fmt.Errorf("compose tool catalog for scope %q: %w", s.id, err)
	}
	return registry, nil
}

func (s *Scope) close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.cancel()
		var errs []error
		if s.mcp != nil {
			errs = append(errs, s.mcp.CloseContext(ctx))
		}
		if s.plugins != nil {
			errs = append(errs, s.plugins.CloseContext(ctx))
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func closeScope(scope *Scope) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()
	return scope.close(ctx)
}

func canonicalScope(workDir string) (string, string, error) {
	if workDir == "" {
		return "", "", nil
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve execution scope %q: %w", workDir, err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("resolve execution scope %q: %w", workDir, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", "", fmt.Errorf("inspect execution scope %q: %w", workDir, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("execution scope %q is not a directory", workDir)
	}
	canonical = filepath.Clean(canonical)
	return canonical, canonical, nil
}

func cloneMCP(in map[string]wingmcp.ServerConfig) map[string]wingmcp.ServerConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]wingmcp.ServerConfig, len(in))
	for key, value := range in {
		value.Command = append([]string(nil), value.Command...)
		value.Environment = cloneStrings(value.Environment)
		value.Headers = cloneStrings(value.Headers)
		out[key] = value
	}
	return out
}

func cloneStrings(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
