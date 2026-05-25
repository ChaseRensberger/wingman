package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chaserensberger/wingman/tool"
)

// LoadError is a non-fatal plugin discovery or startup error.
type LoadError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Status describes one loaded plugin process.
type Status struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Path    string   `json:"path"`
	Tools   []string `json:"tools,omitempty"`
	Running bool     `json:"running"`
	Error   string   `json:"error,omitempty"`
}

// Manager owns discovered external plugin processes.
type Manager struct {
	globalDirs []string

	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	plugins    map[string]*loadedPlugin
	toolOwners map[string]string
	errors     []LoadError
	localDirs  map[string]struct{}
}

type loadedPlugin struct {
	manifest Manifest
	client   *rpcClient
	err      error
}

// New returns a plugin manager and immediately loads global plugin dirs.
func New(ctx context.Context, globalDirs []string) (*Manager, error) {
	m := &Manager{globalDirs: compactDirs(globalDirs)}
	if err := m.Reload(ctx); err != nil {
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

// Reload stops all external plugins and reloads the global plugin set.
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeLocked()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.plugins = make(map[string]*loadedPlugin)
	m.toolOwners = make(map[string]string)
	m.localDirs = make(map[string]struct{})
	m.errors = nil
	m.loadDirsLocked(ctx, m.globalDirs)
	return nil
}

// Close stops all plugin processes.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeLocked()
	return nil
}

// EnsureWorkDir loads project-local plugins for a session working directory.
func (m *Manager) EnsureWorkDir(ctx context.Context, workDir string) {
	local := LocalPluginDir(workDir)
	if local == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.localDirs[local]; ok {
		return
	}
	m.localDirs[local] = struct{}{}
	m.loadDirsLocked(ctx, []string{local})
}

// Status returns loaded plugins and non-fatal load errors.
func (m *Manager) Status() ([]Status, []LoadError) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statuses := make([]Status, 0, len(m.plugins))
	for _, p := range m.plugins {
		status := Status{
			ID:      p.manifest.ID,
			Name:    p.manifest.Name,
			Path:    p.manifest.Path,
			Running: p.client != nil && p.err == nil,
		}
		for _, spec := range p.manifest.Tools {
			status.Tools = append(status.Tools, spec.Name)
		}
		if p.err != nil {
			status.Error = p.err.Error()
		}
		statuses = append(statuses, status)
	}
	errs := append([]LoadError(nil), m.errors...)
	return statuses, errs
}

// Tools returns all currently loaded plugin tools.
func (m *Manager) Tools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]tool.Tool, 0, len(m.toolOwners))
	for _, p := range m.plugins {
		if p.client == nil || p.err != nil {
			continue
		}
		for _, spec := range p.manifest.Tools {
			tools = append(tools, &rpcTool{manager: m, pluginID: p.manifest.ID, spec: spec})
		}
	}
	return tools
}

func (m *Manager) executeToolResult(ctx context.Context, pluginID string, toolName string, params map[string]any, workDir string) (string, map[string]any, error) {
	m.mu.RLock()
	p := m.plugins[pluginID]
	m.mu.RUnlock()
	if p == nil || p.client == nil || p.err != nil {
		return "", nil, fmt.Errorf("plugin %q is not running", pluginID)
	}
	var res toolExecuteResult
	if err := p.client.call(ctx, "tool.execute", toolExecuteParams{Tool: toolName, Params: params, WorkDir: workDir}, &res); err != nil {
		return "", nil, err
	}
	return res.Text, res.Metadata, nil
}

func (m *Manager) loadDirsLocked(ctx context.Context, dirs []string) {
	manifests, errs := discoverManifests(dirs)
	m.errors = append(m.errors, errs...)
	for _, manifest := range manifests {
		if _, exists := m.plugins[manifest.ID]; exists {
			m.errors = append(m.errors, LoadError{Path: manifest.Path, Error: "duplicate plugin id: " + manifest.ID})
			continue
		}
		client, err := startRPC(m.ctx, manifest.Command)
		plugin := &loadedPlugin{manifest: manifest, client: client, err: err}
		m.plugins[manifest.ID] = plugin
		if err != nil {
			m.errors = append(m.errors, LoadError{Path: manifest.Path, Error: err.Error()})
			continue
		}
		for _, spec := range manifest.Tools {
			if owner, exists := m.toolOwners[spec.Name]; exists {
				plugin.err = fmt.Errorf("tool %q already registered by plugin %q", spec.Name, owner)
				m.errors = append(m.errors, LoadError{Path: manifest.Path, Error: plugin.err.Error()})
				break
			}
		}
		if plugin.err != nil {
			_ = client.close()
			continue
		}
		for _, spec := range manifest.Tools {
			m.toolOwners[spec.Name] = manifest.ID
		}
		_ = ctx
	}
}

func (m *Manager) closeLocked() {
	if m.cancel != nil {
		m.cancel()
	}
	var errs []error
	for _, p := range m.plugins {
		if p.client != nil {
			errs = append(errs, p.client.close())
		}
	}
	_ = errors.Join(errs...)
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
	Tool    string         `json:"tool"`
	Params  map[string]any `json:"params"`
	WorkDir string         `json:"work_dir,omitempty"`
}

type toolExecuteResult struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
