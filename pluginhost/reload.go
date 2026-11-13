package pluginhost

import (
	"context"
	"errors"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/tool"
)

// PreparedReload owns a validated replacement until Commit or Abort is called.
type PreparedReload struct {
	manager    *Manager
	generation *generation
	dirs       []string
}

// ValidatePlugins checks the candidate together with bundled plugins before publication.
func (p *PreparedReload) ValidatePlugins(ctx context.Context, bundled ...agentplugin.Plugin) error {
	return p.generation.validatePlugins(ctx, bundled)
}

// PrepareReload validates new directories without changing the current generation.
// The caller must call Commit or Abort before another lifecycle operation.
func (m *Manager) PrepareReload(ctx context.Context, dirs []string) (*PreparedReload, error) {
	m.lifecycleMu.Lock()
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		m.lifecycleMu.Unlock()
		return nil, errors.New("plugin manager is closed")
	}
	generation, err := m.stage(ctx, compactDirs(dirs))
	if err != nil {
		m.lifecycleMu.Unlock()
		return nil, err
	}
	return &PreparedReload{manager: m, generation: generation, dirs: append([]string(nil), dirs...)}, nil
}

// Tools returns the candidate's tool declarations for catalog validation.
func (p *PreparedReload) Tools() []tool.Tool {
	var tools []tool.Tool
	for _, name := range p.generation.toolNames {
		owner := p.generation.toolOwners[name]
		for _, spec := range owner.tools {
			if spec.Name == name {
				tools = append(tools, &rpcTool{plugin: owner, spec: spec})
				break
			}
		}
	}
	return tools
}

// Commit publishes the validated generation.
func (p *PreparedReload) Commit() {
	defer p.manager.lifecycleMu.Unlock()
	p.manager.mu.Lock()
	p.manager.globalDirs = p.dirs
	p.manager.mu.Unlock()
	p.manager.publish(p.generation)
}

// Abort releases a candidate without changing current tools or hooks.
func (p *PreparedReload) Abort() {
	defer p.manager.lifecycleMu.Unlock()
	_ = p.generation.retireWithTimeout()
}
