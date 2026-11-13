package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

var supportedHooks = map[string]bool{
	"before_run": true, "transform_system": true, "transform_context": true, "transform_history": true,
	"transform_tools": true, "transform_params": true, "before_tool": true, "after_tool": true,
	"before_complete": true, "after_run": true, "events": true,
	"turn_start": true, "turn_end": true,
	"prompt":       true,
	"http.request": true, "http.response": true,
}

func validateContributions(id string, c PluginContributions) error {
	seen := map[string]bool{}
	for _, method := range c.Auth {
		if method.ID == "" || method.Integration == "" || method.Label == "" || seen["auth:"+method.ID] {
			return fmt.Errorf("invalid authentication method %q", method.ID)
		}
		if method.Kind != "key" && method.Kind != "oauth" && method.Kind != "command" {
			return fmt.Errorf("invalid authentication kind %q", method.Kind)
		}
		seen["auth:"+method.ID] = true
	}
	for _, part := range c.Parts {
		if !strings.HasPrefix(part, id+".") || seen["part:"+part] {
			return fmt.Errorf("invalid or duplicate part %q", part)
		}
		seen["part:"+part] = true
	}
	if c.ContextBoundary != "" && !seen["part:"+c.ContextBoundary] {
		return fmt.Errorf("context boundary must name a declared plugin part")
	}
	for _, h := range c.Hooks {
		if !supportedHooks[h] || seen[h] {
			return fmt.Errorf("unsupported or duplicate hook %q", h)
		}
		seen[h] = true
	}
	for _, a := range c.Actions {
		if !strings.HasPrefix(a.ID, id+".") || a.Command == "" || seen["action:"+a.ID] {
			return fmt.Errorf("invalid action %q", a.ID)
		}
		seen["action:"+a.ID] = true
		if len(a.InputSchema) > 0 && !json.Valid(a.InputSchema) {
			return fmt.Errorf("invalid action schema %q", a.ID)
		}
	}
	for _, p := range c.Providers {
		if p.Meta.ID == "" || len(p.Models) == 0 || seen["provider:"+p.Meta.ID] {
			return fmt.Errorf("invalid provider %q", p.Meta.ID)
		}
		seen["provider:"+p.Meta.ID] = true
	}
	for _, method := range c.Methods {
		if method.Name == "" || seen["method:"+method.Name] {
			return fmt.Errorf("invalid RPC method %q", method.Name)
		}
		seen["method:"+method.Name] = true
		if err := validateToolSpecs([]ToolSpec{{Name: method.Name, Description: "Plugin RPC", InputSchema: method.InputSchema, OutputSchema: method.OutputSchema}}); err != nil {
			return err
		}
	}
	return nil
}

// Activation pins one executable generation for the lifetime of a run.
type Activation struct {
	manager    *Manager
	generation *generation
	identity   protocol.Identity
	plugins    []*loadedPlugin
	once       sync.Once
	closed     atomic.Bool
}

type activationKey struct{}

func (a *Activation) context(ctx context.Context) context.Context {
	return context.WithValue(WithIdentity(ctx, a.identity), activationKey{}, a)
}

// Acquire pins the current generation before its tools and hooks are resolved.
// Plugins below excludedDirs cannot enter the new activation.
func (m *Manager) Acquire(identity protocol.Identity, excludedDirs ...string) (*Activation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("plugin manager is closed")
	}
	a := &Activation{manager: m, generation: m.generation, identity: identity}
	if a.generation == nil {
		return a, nil
	}
	ids := make([]string, 0, len(a.generation.plugins))
	for id := range a.generation.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := a.generation.plugins[id]
		excluded := false
		for _, dir := range excludedDirs {
			relative, err := filepath.Rel(dir, p.manifest.Path)
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		if err := p.acquire(); err != nil {
			for _, previous := range a.plugins {
				previous.mu.Lock()
				previous.pins--
				previous.mu.Unlock()
				previous.release()
			}
			return nil, err
		}
		p.mu.Lock()
		p.pins++
		p.mu.Unlock()
		a.plugins = append(a.plugins, p)
	}
	a.generation.leases++
	return a, nil
}

// Close releases the run's generation and permits retired processes to stop.
func (a *Activation) Close(context.Context) error {
	a.once.Do(func() {
		a.closed.Store(true)
		for _, p := range a.plugins {
			p.mu.Lock()
			p.pins--
			p.mu.Unlock()
			p.release()
		}
		a.manager.mu.Lock()
		defer a.manager.mu.Unlock()
		if a.generation != nil {
			a.generation.leases--
			if a.generation.retiring && a.generation.leases == 0 {
				close(a.generation.drained)
			}
		}
	})
	return nil
}

// Tools returns the executable tools captured by this activation.
func (a *Activation) Tools() []tool.Tool {
	var tools []tool.Tool
	for _, p := range a.plugins {
		for _, spec := range p.tools {
			tools = append(tools, &rpcTool{plugin: p, spec: spec, pinned: true, activation: a})
		}
	}
	return tools
}

// Plugins adapts external contributions to the existing Go hook pipeline.
func (a *Activation) Plugins() []agentplugin.Plugin {
	var plugins []agentplugin.Plugin
	for _, p := range a.plugins {
		plugins = append(plugins, &externalPlugin{activation: a, plugin: p})
	}
	return plugins
}

type externalPlugin struct {
	activation *Activation
	plugin     *loadedPlugin
}

func (p *externalPlugin) Name() string { return p.plugin.id.ID }

func (p *externalPlugin) call(ctx context.Context, input protocol.Hook) (protocol.Hook, error) {
	if p.activation.closed.Load() {
		return protocol.Hook{}, fmt.Errorf("plugin activation is closed")
	}
	input.Identity = p.activation.identity
	ctx = p.activation.context(ctx)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := p.plugin.acquirePinned(true); err != nil {
		return protocol.Hook{}, err
	}
	defer p.plugin.release()
	var output protocol.Hook
	err := p.plugin.client.call(ctx, "hook.execute", input, &output)
	return output, err
}

func (p *externalPlugin) Activate(r *agentplugin.Registry) (agentplugin.Cleanup, error) {
	for _, part := range p.plugin.contributions.Parts {
		if err := r.RegisterPart(part, func(raw []byte) (models.Part, error) {
			var part models.OpaquePart
			err := json.Unmarshal(raw, &part)
			return part, err
		}); err != nil {
			return nil, err
		}
	}
	if boundary := p.plugin.contributions.ContextBoundary; boundary != "" {
		if err := r.RegisterContextBoundary(boundary); err != nil {
			return nil, err
		}
	}
	for _, name := range p.plugin.contributions.Hooks {
		var err error
		switch name {
		case "turn_start":
			err = r.RegisterOnTurnStart(func(ctx context.Context, step int) error {
				_, err := p.call(ctx, protocol.Hook{Name: name, Step: step})
				return err
			})
		case "turn_end":
			err = r.RegisterOnTurnEnd(func(ctx context.Context, step int, turn run.Turn) error {
				turn.Failure = nil
				_, err := p.call(ctx, protocol.Hook{Name: name, Step: step, Turn: turn})
				return err
			})
		case "before_run":
			err = r.RegisterBeforeRun(func(ctx context.Context, messages []models.Message) ([]models.Message, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Messages: messages})
				return out.Messages, err
			})
		case "transform_system":
			err = r.RegisterTransformSystem(func(ctx context.Context, info run.TransformSystemInfo) (string, error) {
				definitions := make([]models.ToolDef, 0, len(info.Tools))
				for _, t := range info.Tools {
					definitions = append(definitions, t.Definition().AsModelToolDef())
				}
				out, err := p.call(ctx, protocol.Hook{Name: name, System: info.System, Model: info.Model, ModelInfo: info.ModelInfo, Permissions: info.Permissions, Tools: definitions})
				return out.System, err
			})
		case "transform_context":
			err = r.RegisterTransformContext(func(ctx context.Context, info run.TransformContextInfo) ([]models.Message, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Messages: info.Messages, Step: info.Step, Model: info.Model, ModelInfo: info.ModelInfo})
				return out.Messages, err
			})
		case "transform_history":
			err = r.RegisterTransformHistory(func(ctx context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
				if info.Client != nil {
					ctx = context.WithValue(ctx, modelClientKey{}, info.Client)
				}
				if info.Sink != nil {
					ctx = context.WithValue(ctx, eventSinkKey{}, info.Sink)
				}
				out, err := p.call(ctx, protocol.Hook{Name: name, Messages: info.Messages, Step: info.Step, Model: info.Model, ModelInfo: info.ModelInfo, Request: info.Request, Usage: info.Usage, ContextOverflow: info.ContextOverflow})
				return out.Messages, err
			})
		case "transform_tools":
			err = r.RegisterTransformToolDefs(func(ctx context.Context, info run.TransformToolDefsInfo) ([]models.ToolDef, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Tools: info.Tools, Step: info.Step, Model: info.Model, ModelInfo: info.ModelInfo})
				return out.Tools, err
			})
		case "transform_params":
			err = r.RegisterTransformParams(func(ctx context.Context, info run.TransformParamsInfo) (run.TransformParamsResult, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Params: info.Params, Step: info.Step, Model: info.Model, ModelInfo: info.ModelInfo})
				return run.TransformParamsResult{Params: out.Params}, err
			})
		case "before_tool":
			err = r.RegisterBeforeToolCall(func(ctx context.Context, call run.ToolCall) (map[string]any, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Call: call})
				if err == nil && out.Skip != "" {
					err = fmt.Errorf("%s: %w", out.Skip, run.ErrSkipTool)
				}
				return out.Call.Args, err
			})
		case "after_tool":
			err = r.RegisterAfterToolCall(func(ctx context.Context, call run.ToolCall, result run.ToolResult) (run.ToolResult, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Call: call, Result: result})
				return out.Result, err
			})
		case "before_complete":
			err = r.RegisterBeforeComplete(func(ctx context.Context, info run.CompletionInfo) (run.CompletionDecision, error) {
				out, err := p.call(ctx, protocol.Hook{Name: name, Candidate: &info.Candidate, Messages: info.Messages, Step: info.Step, Usage: info.Usage, Continuations: info.Continuations})
				return out.Decision, err
			})
		case "after_run":
			err = r.RegisterAfterRun(func(ctx context.Context, info run.AfterRunInfo) error {
				info.Result.Turns = append([]run.Turn(nil), info.Result.Turns...)
				for i := range info.Result.Turns {
					info.Result.Turns[i].Failure = nil
				}
				input := protocol.Hook{Name: name, RunResult: info.Result}
				if info.Err != nil {
					input.Error = info.Err.Error()
				}
				_, err := p.call(ctx, input)
				return err
			})
		case "events":
			err = r.RegisterSink(&externalSink{plugin: p})
		}
		if err != nil {
			return nil, err
		}
	}
	for _, action := range p.plugin.contributions.Actions {
		if err := r.RegisterAction(agentplugin.Action{ID: action.ID, Command: action.Command, Description: action.Description, InputSchema: action.InputSchema, Handler: func(ctx context.Context, info agentplugin.ActionInfo) error {
			if info.Client != nil {
				ctx = context.WithValue(ctx, modelClientKey{}, info.Client)
			}
			if info.Sink != nil {
				ctx = context.WithValue(ctx, eventSinkKey{}, info.Sink)
			}
			return p.activation.Call(ctx, p.Name(), "action.execute", protocol.ActionCall{Name: action.ID, Input: info.Input, Identity: p.activation.identity, History: info.History, Model: info.Model, ModelInfo: info.ModelInfo}, nil)
		}}); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

type externalSink struct{ plugin *externalPlugin }

func (s *externalSink) OnEvent(event run.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(WithIdentity(context.Background(), s.plugin.activation.identity), time.Second)
	defer cancel()
	_ = s.plugin.activation.Call(ctx, s.plugin.Name(), "plugin.event", protocol.Event{Identity: s.plugin.activation.identity, Type: fmt.Sprintf("%T", event), Data: data}, nil)
}

// Call invokes a method in the captured executable generation.
func (a *Activation) Call(ctx context.Context, pluginID, method string, input, output any) error {
	if a.closed.Load() {
		return fmt.Errorf("plugin activation is closed")
	}
	for _, p := range a.plugins {
		if p.id.ID == pluginID {
			if err := p.acquirePinned(true); err != nil {
				return err
			}
			defer p.release()
			return p.client.call(a.context(ctx), method, input, output)
		}
	}
	return fmt.Errorf("plugin %q is not active", pluginID)
}

// Methods returns the custom operations declared by this activation.
func (a *Activation) Methods() map[string][]protocol.Method {
	out := map[string][]protocol.Method{}
	for _, p := range a.plugins {
		out[p.id.ID] = append([]protocol.Method(nil), p.contributions.Methods...)
	}
	return out
}
