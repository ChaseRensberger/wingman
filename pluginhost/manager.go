package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/tool"
)

const (
	initializeTimeout = 10 * time.Second
	healthTimeout     = 2 * time.Second
	shutdownTimeout   = 10 * time.Second
	healthInterval    = 30 * time.Second
)

// LoadError is a plugin generation or retirement error.
type LoadError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Status describes one loaded plugin process.
type Status struct {
	ID              string              `json:"id"`
	Name            string              `json:"name,omitempty"`
	Path            string              `json:"path"`
	Tools           []string            `json:"tools,omitempty"`
	Running         bool                `json:"running"`
	Error           string              `json:"error,omitempty"`
	ProtocolVersion int                 `json:"protocol_version,omitempty"`
	PluginVersion   string              `json:"plugin_version,omitempty"`
	Capabilities    []string            `json:"capabilities,omitempty"`
	Status          string              `json:"status"`
	PID             int                 `json:"pid,omitempty"`
	StartedAt       time.Time           `json:"started_at,omitempty"`
	ExitedAt        time.Time           `json:"exited_at,omitempty"`
	LastHealthAt    time.Time           `json:"last_health_at,omitempty"`
	HealthMessage   string              `json:"health_message,omitempty"`
	Diagnostics     []Diagnostic        `json:"diagnostics,omitempty"`
	Contributions   PluginContributions `json:"contributions"`
}

// Manager owns the current, atomically published plugin generation.
type Manager struct {
	globalDirs []string

	mu          sync.RWMutex
	generation  *generation
	errors      []LoadError
	closed      bool
	retired     []*generation
	rootCtx     context.Context
	rootCancel  context.CancelFunc
	hostHandler func(context.Context, string, json.RawMessage) (any, error)
	lifecycleMu sync.Mutex
}

type generation struct {
	ctx        context.Context
	cancel     context.CancelFunc
	plugins    map[string]*loadedPlugin
	toolOwners map[string]*loadedPlugin
	toolNames  []string
	leases     int
	drained    chan struct{}
	retiring   bool
	closeOnce  sync.Once
	closeErr   error
}

type loadedPlugin struct {
	manifest        Manifest
	id              InitializedPlugin
	protocolVersion int
	capabilities    []string
	tools           []ToolSpec
	contributions   PluginContributions
	client          *rpcClient

	mu            sync.Mutex
	activeCalls   int
	pins          int
	idle          chan struct{}
	retiring      bool
	status        string
	err           error
	startedAt     time.Time
	exitedAt      time.Time
	lastHealthAt  time.Time
	healthMessage string
	healthStop    chan struct{}
	healthOnce    sync.Once
}

// New returns a plugin manager and immediately stages global plugin dirs.
func New(ctx context.Context, globalDirs []string) (*Manager, error) {
	rootCtx, rootCancel := context.WithCancel(ctx)
	m := &Manager{
		globalDirs: compactDirs(globalDirs),
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}
	if err := m.reload(ctx); err != nil {
		rootCancel()
		return nil, err
	}
	return m, nil
}

// DefaultGlobalDir returns Wingman's default global plugin directory.
func DefaultGlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wingman", "plugins"), nil
}

// Reload stages every plugin directory owned by this execution scope.
func (m *Manager) Reload(ctx context.Context) error {
	return m.reload(ctx)
}

func (m *Manager) reload(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return errors.New("plugin manager is closed")
	}
	dirs := append([]string(nil), m.globalDirs...)
	m.mu.RUnlock()
	candidate, err := m.stage(ctx, dirs)
	if err != nil {
		return err
	}
	m.publish(candidate)
	return nil
}

func (m *Manager) publish(candidate *generation) {
	m.mu.Lock()
	old := m.generation
	m.generation = candidate
	m.errors = nil
	deferred := old != nil && old.leases > 0
	if deferred {
		old.retiring = true
		m.retired = append(m.retired, old)
		for _, p := range old.plugins {
			p.mu.Lock()
			p.retiring = true
			p.mu.Unlock()
		}
	}
	m.mu.Unlock()

	if deferred {
		go func() {
			select {
			case <-old.drained:
			case <-m.rootCtx.Done():
			}
			if err := old.retireWithTimeout(); err != nil {
				m.addError(LoadError{Path: "plugin retirement", Error: err.Error()})
			}
			m.mu.Lock()
			for i, retired := range m.retired {
				if retired == old {
					m.retired = append(m.retired[:i], m.retired[i+1:]...)
					break
				}
			}
			m.mu.Unlock()
		}()
	} else if old != nil {
		if err := old.retireWithTimeout(); err != nil {
			m.addError(LoadError{Path: "plugin retirement", Error: err.Error()})
		}
	}
}

// Close gracefully retires all plugin processes and cancels the manager root.
func (m *Manager) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return m.CloseContext(ctx)
}

// CloseContext retires all plugin processes within the caller's deadline.
func (m *Manager) CloseContext(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	current := m.generation
	retired := append([]*generation(nil), m.retired...)
	m.mu.Unlock()

	var err error
	if current != nil {
		err = current.retire(ctx)
		if err != nil {
			m.addError(LoadError{Path: "plugin retirement", Error: err.Error()})
		}
	}
	for _, g := range retired {
		err = errors.Join(err, g.retire(ctx))
	}
	m.rootCancel()
	return err
}

// Status returns loaded plugins and generation/retirement errors.
func (m *Manager) Status() ([]Status, []LoadError) {
	m.mu.RLock()
	current := m.generation
	errs := append([]LoadError(nil), m.errors...)
	m.mu.RUnlock()
	if current == nil {
		return nil, errs
	}
	ids := make([]string, 0, len(current.plugins))
	for id := range current.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	statuses := make([]Status, 0, len(ids))
	for _, id := range ids {
		statuses = append(statuses, current.plugins[id].snapshot())
	}
	return statuses, errs
}

// Tools returns generation-bound plugin tools in deterministic name order.
func (m *Manager) Tools() []tool.Tool {
	m.mu.RLock()
	current := m.generation
	m.mu.RUnlock()
	if current == nil {
		return nil
	}
	tools := make([]tool.Tool, 0, len(current.toolNames))
	for _, name := range current.toolNames {
		plugin := current.toolOwners[name]
		if !plugin.available() {
			continue
		}
		for _, spec := range plugin.tools {
			if spec.Name == name {
				tools = append(tools, &rpcTool{plugin: plugin, spec: spec})
				break
			}
		}
	}
	return tools
}

func (p *loadedPlugin) available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.client.Done():
		p.noteExitLocked()
	default:
	}
	return !p.retiring && (p.status == "running" || p.status == "degraded")
}

func (m *Manager) stage(ctx context.Context, dirs []string) (*generation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifests, discoveryErrors := discoverManifests(dirs)
	if len(discoveryErrors) > 0 {
		return nil, fmt.Errorf("discover plugins: %s", discoveryErrors[0].Error)
	}
	gctx, cancel := context.WithCancel(m.rootCtx)
	g := &generation{ctx: gctx, cancel: cancel, plugins: make(map[string]*loadedPlugin), toolOwners: make(map[string]*loadedPlugin), drained: make(chan struct{})}
	fail := func(err error) (*generation, error) {
		_ = g.retireWithTimeout()
		return nil, err
	}
	for _, manifest := range manifests {
		if manifest.Disabled {
			continue
		}
		if _, exists := g.plugins[manifest.ID]; exists {
			return fail(fmt.Errorf("duplicate plugin id: %s", manifest.ID))
		}
		plugin, err := stagePlugin(ctx, gctx, manifest)
		if err != nil {
			return fail(fmt.Errorf("plugin %q: %w", manifest.ID, err))
		}
		g.plugins[manifest.ID] = plugin
		m.mu.RLock()
		handler := m.hostHandler
		m.mu.RUnlock()
		plugin.client.mu.Lock()
		plugin.client.hostHandler = bindHost(plugin.id.ID, handler)
		plugin.client.mu.Unlock()
	}
	owners := make(map[string]string)
	for id, plugin := range g.plugins {
		for _, spec := range plugin.tools {
			if owner, exists := owners[spec.Name]; exists {
				return fail(fmt.Errorf("tool %q already registered by plugin %q", spec.Name, owner))
			}
			owners[spec.Name] = id
			g.toolOwners[spec.Name] = plugin
			g.toolNames = append(g.toolNames, spec.Name)
		}
	}
	sort.Strings(g.toolNames)
	for _, plugin := range g.plugins {
		select {
		case <-plugin.client.Done():
			return fail(fmt.Errorf("plugin %q exited during staging: %w", plugin.id.ID, plugin.client.Err()))
		default:
		}
		plugin.startHealthLoop(gctx)
	}
	base, err := provider.NewRegistry(nil)
	if err != nil {
		return fail(err)
	}
	activation := &Activation{}
	ids := make([]string, 0, len(g.plugins))
	for id := range g.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		activation.plugins = append(activation.plugins, g.plugins[id])
	}
	if _, err := base.WithRegistrations(activation.Providers()); err != nil {
		return fail(err)
	}
	if err := g.validatePlugins(ctx, nil); err != nil {
		return fail(err)
	}
	return g, nil
}

// ValidatePlugins checks the current contributions together with bundled plugins.
func (m *Manager) ValidatePlugins(ctx context.Context, bundled ...agentplugin.Plugin) error {
	m.mu.RLock()
	g := m.generation
	m.mu.RUnlock()
	if g == nil {
		return nil
	}
	return g.validatePlugins(ctx, bundled)
}

func (g *generation) validatePlugins(ctx context.Context, bundled []agentplugin.Plugin) error {
	ids := make([]string, 0, len(g.plugins))
	for id := range g.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	a := &Activation{}
	for _, id := range ids {
		a.plugins = append(a.plugins, g.plugins[id])
	}
	plugins := append(append([]agentplugin.Plugin(nil), bundled...), a.Plugins()...)
	if len(plugins) == 0 {
		return nil
	}
	contributions, err := agentplugin.ActivateAll(plugins...)
	if err != nil {
		return err
	}
	return contributions.Close(ctx)
}

func stagePlugin(ctx, generationCtx context.Context, manifest Manifest) (*loadedPlugin, error) {
	client, err := startRPC(generationCtx, manifest.Command)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*loadedPlugin, error) {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = client.Close(closeCtx)
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()
	var initialized InitializeResult
	err = client.call(initCtx, InitializeMethod, InitializeParams{
		HostName:                   "Wingman",
		HostVersion:                hostBuildVersion(),
		SupportedProtocolVersions:  []int{ProtocolVersion},
		SupportedContributionKinds: []string{ContributionTools, "hooks", "actions", "providers", "methods", "parts", "transforms", "auth"},
		Plugin:                     map[string]any{"id": manifest.ID, "name": manifest.Name, "config": manifest.Config},
	}, &initialized)
	if err != nil {
		return fail(fmt.Errorf("initialize: %w", err))
	}
	if initialized.ProtocolVersion != ProtocolVersion {
		return fail(fmt.Errorf("unsupported protocol version %d", initialized.ProtocolVersion))
	}
	if initialized.Plugin.ID == "" || initialized.Plugin.ID != manifest.ID {
		return fail(fmt.Errorf("initialized plugin id %q does not match manifest id %q", initialized.Plugin.ID, manifest.ID))
	}
	if err := validateCapabilities(initialized.Capabilities); err != nil {
		return fail(err)
	}
	if err := validateToolSpecs(initialized.Contributions.Tools); err != nil {
		return fail(err)
	}
	if err := validateContributions(manifest.ID, initialized.Contributions); err != nil {
		return fail(err)
	}
	client.SetCapabilities(initialized.Capabilities)
	p := &loadedPlugin{
		manifest: manifest, id: initialized.Plugin, protocolVersion: initialized.ProtocolVersion, contributions: initialized.Contributions,
		capabilities: append([]string(nil), initialized.Capabilities...), tools: append([]ToolSpec(nil), initialized.Contributions.Tools...),
		client: client, status: "running", startedAt: client.StartedAt(), idle: closedSignal(), healthStop: make(chan struct{}),
	}
	if hasCapability(initialized.Capabilities, CapabilityHealth) {
		if err := p.checkHealth(ctx); err != nil {
			return fail(err)
		}
	}
	go p.watchExit()
	return p, nil
}

func hostBuildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}

func validateCapabilities(capabilities []string) error {
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		switch capability {
		case CapabilityCancellation, CapabilityProgress, CapabilityHealth:
		default:
			return fmt.Errorf("unsupported capability %q", capability)
		}
	}
	return nil
}

func hasCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

func (p *loadedPlugin) startHealthLoop(ctx context.Context) {
	if !hasCapability(p.capabilities, CapabilityHealth) {
		return
	}
	go func() {
		ticker := time.NewTicker(healthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-p.healthStop:
				return
			case <-ticker.C:
				_ = p.checkHealth(ctx)
			}
		}
	}()
}

func (p *loadedPlugin) checkHealth(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, healthTimeout)
	defer cancel()
	var health HealthResult
	err := p.client.call(ctx, HealthMethod, nil, &health)
	p.mu.Lock()
	p.lastHealthAt = time.Now()
	if err != nil {
		p.healthMessage = err.Error()
		if p.status == "running" {
			p.status = "degraded"
		}
	} else {
		p.healthMessage = health.Message
		if health.Status == "ok" && p.status == "degraded" {
			p.status = "running"
		} else if health.Status != "ok" && p.status == "running" {
			p.status = "degraded"
		}
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	if health.Status != "ok" {
		return fmt.Errorf("health status %q", health.Status)
	}
	return nil
}

func (p *loadedPlugin) watchExit() {
	<-p.client.Done()
	p.mu.Lock()
	p.noteExitLocked()
	p.mu.Unlock()
}

func (p *loadedPlugin) noteExitLocked() {
	if !p.exitedAt.IsZero() {
		return
	}
	p.exitedAt = time.Now()
	if p.retiring {
		return
	}
	p.err = p.client.Err()
	if p.err == nil {
		p.status = "stopped"
		slog.Default().Info("plugin process stopped", "plugin_id", p.id.ID)
	} else {
		p.status = "failed"
		slog.Default().Error("plugin process exited", "plugin_id", p.id.ID, "error", p.err)
	}
}

func (p *loadedPlugin) executeTool(ctx context.Context, name string, inv tool.Invocation) (tool.Result, error) {
	return p.executeToolPinned(ctx, name, inv, false)
}

func (p *loadedPlugin) executeToolPinned(ctx context.Context, name string, inv tool.Invocation, pinned bool) (tool.Result, error) {
	if err := p.acquirePinned(pinned); err != nil {
		return tool.Result{}, err
	}
	defer p.release()
	var result toolExecuteResult
	var progress func(ToolProgressParams)
	if hasCapability(p.capabilities, CapabilityProgress) {
		progress = func(update ToolProgressParams) {
			inv.Progress.Report(update.OutputDelta, update.Metadata)
		}
	}
	err := p.client.callWithProgress(ctx, "tool.execute", toolExecuteParams{
		Tool:  name,
		Input: inv.Input,
		Context: toolExecutionContext{
			SessionID: inv.SessionID, RunID: inv.RunID, AgentID: inv.AgentID,
			ToolUseID: inv.ToolUseID, CallID: inv.CallID, MessageID: inv.MessageID,
			PartID: inv.PartID, ModelCallID: inv.ModelCallID, WorkDir: inv.WorkDir,
		},
	}, &result, progress)
	if err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Text: result.Text, OutputParts: result.OutputParts, Structured: result.Structured, Metadata: result.Metadata}, nil
}

func (p *loadedPlugin) acquire() error {
	return p.acquirePinned(false)
}

func (p *loadedPlugin) acquirePinned(pinned bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retiring && (!pinned || p.pins == 0) {
		return fmt.Errorf("plugin %q is retiring", p.id.ID)
	}
	if p.status == "failed" || p.status == "stopped" {
		return fmt.Errorf("plugin %q is not running", p.id.ID)
	}
	if p.activeCalls == 0 {
		p.idle = make(chan struct{})
	}
	p.activeCalls++
	return nil
}

func (p *loadedPlugin) release() {
	p.mu.Lock()
	p.activeCalls--
	if p.activeCalls == 0 {
		close(p.idle)
	}
	p.mu.Unlock()
}

func (g *generation) retireWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return g.retire(ctx)
}

func (g *generation) retire(ctx context.Context) error {
	g.closeOnce.Do(func() { g.closeErr = g.close(ctx) })
	return g.closeErr
}

func (g *generation) close(ctx context.Context) error {
	ids := make([]string, 0, len(g.plugins))
	for id := range g.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g.plugins[id].stopHealth()
	}
	var errs []error
	for _, id := range ids {
		if err := g.plugins[id].retire(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	g.cancel()
	return errors.Join(errs...)
}

func (p *loadedPlugin) stopHealth() {
	p.healthOnce.Do(func() { close(p.healthStop) })
}

func (p *loadedPlugin) retire(ctx context.Context) error {
	p.mu.Lock()
	p.retiring = true
	idle := p.idle
	p.mu.Unlock()
	var drainErr error
	select {
	case <-idle:
	case <-ctx.Done():
		drainErr = fmt.Errorf("plugin %q active calls did not drain: %w", p.id.ID, ctx.Err())
	}
	closeErr := p.client.Close(ctx)
	p.mu.Lock()
	p.exitedAt = time.Now()
	p.status = "stopped"
	if closeErr != nil && !errors.Is(closeErr, context.DeadlineExceeded) {
		p.err = closeErr
	}
	p.mu.Unlock()
	return errors.Join(drainErr, closeErr)
}

func (p *loadedPlugin) snapshot() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.client.Done():
		p.noteExitLocked()
	default:
	}
	tools := make([]string, len(p.tools))
	for i, spec := range p.tools {
		tools[i] = spec.Name
	}
	sort.Strings(tools)
	capabilities := append([]string(nil), p.capabilities...)
	sort.Strings(capabilities)
	name := p.id.Name
	if name == "" {
		name = p.manifest.Name
	}
	status := Status{ID: p.id.ID, Name: name, Path: p.manifest.Path, Tools: tools, ProtocolVersion: p.protocolVersion, Contributions: p.contributions,
		PluginVersion: p.id.Version, Capabilities: capabilities, Status: p.status, PID: p.client.PID(), StartedAt: p.startedAt,
		ExitedAt: p.exitedAt, LastHealthAt: p.lastHealthAt, HealthMessage: p.healthMessage, Diagnostics: p.client.Diagnostics()}
	status.Running = status.Status == "running" || status.Status == "degraded"
	if p.err != nil {
		status.Error = p.err.Error()
	}
	return status
}

func closedSignal() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (m *Manager) addError(err LoadError) {
	m.mu.Lock()
	m.errors = append(m.errors, err)
	m.mu.Unlock()
}

func compactDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		clean := filepath.Clean(dir)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

type toolExecuteParams struct {
	Tool    string               `json:"tool"`
	Input   map[string]any       `json:"input"`
	Context toolExecutionContext `json:"context"`
}

type toolExecutionContext struct {
	SessionID   string `json:"session_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	CallID      string `json:"call_id,omitempty"`
	MessageID   string `json:"message_id,omitempty"`
	PartID      string `json:"part_id,omitempty"`
	ModelCallID string `json:"model_call_id,omitempty"`
	WorkDir     string `json:"work_dir,omitempty"`
}

type toolExecuteResult struct {
	Text        string         `json:"text"`
	OutputParts models.Content `json:"output_parts,omitempty"`
	Structured  any            `json:"structured,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
