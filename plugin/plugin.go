// Package plugin is the Go SDK for independently installed Wingman plugins.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	agentplugin "github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/internal/pluginrpc"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

// Definition identifies a plugin and constructs its process-owned resources.
type Definition struct {
	ID      string
	Name    string
	Version string
	Setup   func(context.Context, *Host, map[string]any) (*Extension, error)
}

// Extension contains the handlers registered by one plugin process.
type Extension struct {
	Tools           []Tool
	Hooks           run.Hooks
	Actions         []Action
	Providers       []Provider
	Methods         []Method
	Parts           []string
	ContextBoundary string
	OnEvent         func(context.Context, protocol.Event) error
	OnPrompt        func(context.Context, protocol.Prompt) (string, error)
	Transforms      *Transforms
	HTTP            HTTPHooks
	Authentication  []Authentication
	Close           func(context.Context) error
	host            *Host
}

// Tool combines a tool declaration and its executor.
type Tool struct {
	protocol.Tool
	Execute func(context.Context, tool.Invocation) (tool.Result, error)
}

// Action combines a session-command declaration and its executor.
type Action struct {
	protocol.Action
	Execute agentplugin.ActionHandler
}

// Provider combines a provider declaration and its streaming implementation.
type Provider struct {
	protocol.Provider
	Prepare func(context.Context, protocol.ProviderCall) (*models.PreparedRequest, error)
	Stream  func(context.Context, protocol.ProviderCall, func(models.StreamPart) error) (*models.Message, error)
}

// Method combines a custom RPC declaration and its executor.
type Method struct {
	protocol.Method
	Execute func(context.Context, json.RawMessage) (any, error)
}

// Host calls Wingman without exposing server credentials to the plugin.
type Host struct {
	peer *pluginrpc.Peer
	ctx  context.Context
}
type scopeKey struct{}

// Background grants background work the current client scope until the plugin unloads.
// The returned context does not inherit the invocation's cancellation.
func (h *Host) Background(ctx context.Context) (context.Context, error) {
	var token string
	if err := h.Call(ctx, "scope.bind", nil, &token); err != nil {
		return nil, err
	}
	return context.WithValue(h.ctx, scopeKey{}, token), nil
}

// Call invokes a host operation in the current execution's client context.
func (h *Host) Call(ctx context.Context, method string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	token, _ := ctx.Value(scopeKey{}).(string)
	return h.peer.Call(ctx, "host.call", protocol.HostCall{InvocationID: pluginrpc.RequestID(ctx), ScopeToken: token, Method: method, Input: data}, output)
}

// Request uses Wingman's normal API with the current invocation's client identity.
func (h *Host) Request(ctx context.Context, method, path string, body, output any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var result protocol.HTTPResponse
	if err := h.Call(ctx, "api.request", protocol.HTTPRequest{Method: method, Path: path, Body: data}, &result); err != nil {
		return err
	}
	if result.Status < 200 || result.Status >= 300 {
		return fmt.Errorf("Wingman API returned %d: %s", result.Status, result.Body)
	}
	if output != nil {
		return json.Unmarshal(result.Body, output)
	}
	return nil
}

// StorageGet reads a durable value scoped to this plugin, client, and directory.
func (h *Host) StorageGet(ctx context.Context, key string, output any) error {
	return h.Call(ctx, "storage.get", map[string]any{"key": key}, output)
}

// StorageSet stores a durable JSON value in this plugin's execution scope.
func (h *Host) StorageSet(ctx context.Context, key string, value any) error {
	return h.Call(ctx, "storage.set", map[string]any{"key": key, "value": value}, nil)
}

// StorageRemove removes a value from this plugin's execution scope.
func (h *Host) StorageRemove(ctx context.Context, key string) error {
	return h.Call(ctx, "storage.remove", map[string]any{"key": key}, nil)
}

// Serve runs a plugin on standard input and output. Diagnostics belong on stderr.
func Serve(ctx context.Context, definition Definition) error {
	return ServeIO(ctx, definition, os.Stdin, os.Stdout)
}

// ServeIO runs a plugin on an owned connection.
func ServeIO(ctx context.Context, definition Definition, reader io.ReadCloser, writer io.Writer) error {
	if definition.ID == "" || definition.Setup == nil {
		return errors.New("plugin requires an ID and Setup")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	workCtx, stopWork := context.WithCancel(ctx)
	defer stopWork()
	var mu sync.RWMutex
	var extension *Extension
	var initialized bool
	var closeOnce sync.Once
	var closeErr error
	bodies := &httpBodies{}
	var peer *pluginrpc.Peer
	closeExtension := func(ctx context.Context) error {
		closeOnce.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cleanupCancel()
			stopWork()
			bodies.close()
			mu.RLock()
			e := extension
			mu.RUnlock()
			if e != nil && e.Close != nil {
				closeErr = e.Close(cleanupCtx)
			}
		})
		return closeErr
	}
	peer = pluginrpc.New(writer, func(callCtx context.Context, method string, raw json.RawMessage) (any, error) {
		if method == "plugin.initialize" {
			mu.Lock()
			if initialized {
				mu.Unlock()
				return nil, errors.New("plugin already initialized")
			}
			initialized = true
			mu.Unlock()
			var input struct {
				SupportedProtocolVersions []int `json:"supported_protocol_versions"`
				Plugin                    struct {
					ID     string         `json:"id"`
					Config map[string]any `json:"config"`
				} `json:"plugin"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if input.Plugin.ID != definition.ID {
				return nil, errors.New("plugin identity does not match")
			}
			compatible := false
			for _, version := range input.SupportedProtocolVersions {
				compatible = compatible || version == protocol.Version
			}
			if !compatible {
				return nil, errors.New("unsupported plugin protocol")
			}
			e, err := definition.Setup(workCtx, &Host{peer: peer, ctx: workCtx}, input.Plugin.Config)
			if err != nil {
				return nil, err
			}
			if e == nil {
				return nil, errors.New("Setup returned no extension")
			}
			e.host = &Host{peer: peer, ctx: workCtx}
			mu.Lock()
			extension = e
			mu.Unlock()
			contributions, err := e.contributions()
			if err != nil {
				return nil, err
			}
			return map[string]any{"protocol_version": protocol.Version, "plugin": map[string]string{"id": definition.ID, "name": definition.Name, "version": definition.Version}, "capabilities": []string{"health", "cancellation", "progress"}, "contributions": contributions}, nil
		}
		mu.RLock()
		e := extension
		mu.RUnlock()
		if e == nil {
			return nil, errors.New("plugin is not initialized")
		}
		switch method {
		case "auth.execute":
			var input protocol.AuthCall
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			for _, auth := range e.Authentication {
				if auth.ID == input.Method {
					return auth.Handle(callCtx, input)
				}
			}
			return nil, errors.New("authentication method is not registered")
		case "http.request", "http.response", "http.body.read", "http.body.close":
			return bodies.call(callCtx, e, method, raw)
		case "plugin.health":
			return map[string]string{"status": "ok"}, nil
		case "plugin.shutdown":
			return map[string]string{"message": "stopped"}, closeExtension(callCtx)
		case "tool.execute":
			var input struct {
				Tool    string         `json:"tool"`
				Input   map[string]any `json:"input"`
				Context struct {
					protocol.Identity
					ToolUseID   string `json:"tool_use_id"`
					CallID      string `json:"call_id"`
					MessageID   string `json:"message_id"`
					PartID      string `json:"part_id"`
					ModelCallID string `json:"model_call_id"`
				} `json:"context"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			for _, t := range e.Tools {
				if t.Name == input.Tool {
					return t.Execute(callCtx, tool.Invocation{Input: input.Input, WorkDir: input.Context.WorkDir, SessionID: input.Context.SessionID, RunID: input.Context.RunID, AgentID: input.Context.AgentID, ToolUseID: input.Context.ToolUseID, CallID: input.Context.CallID, MessageID: input.Context.MessageID, PartID: input.Context.PartID, ModelCallID: input.Context.ModelCallID, Progress: tool.NewProgress(func(delta string, metadata map[string]any) {
						_ = peer.Notify("tool.progress", map[string]any{"request_id": pluginrpc.RequestID(callCtx), "output_delta": delta, "metadata": metadata})
					})})
				}
			}
		case "hook.execute":
			var hook protocol.Hook
			if err := json.Unmarshal(raw, &hook); err != nil {
				return nil, err
			}
			return e.hook(callCtx, hook)
		case "action.execute", "rpc.execute":
			var input struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if method == "action.execute" {
				var call protocol.ActionCall
				if err := json.Unmarshal(raw, &call); err != nil {
					return nil, err
				}
				for _, action := range e.Actions {
					if action.ID == input.Name {
						return nil, action.Execute(callCtx, agentplugin.ActionInfo{SessionID: call.Identity.SessionID, RunID: call.Identity.RunID, Input: call.Input, History: call.History, Model: call.Model, ModelInfo: call.ModelInfo, Client: &hostModelClient{host: e.host}, Sink: &hostSink{host: e.host, ctx: callCtx}})
					}
				}
			}
			if method == "rpc.execute" {
				for _, operation := range e.Methods {
					if operation.Name == input.Name {
						return operation.Execute(callCtx, input.Input)
					}
				}
			}
		case "plugin.event":
			var event protocol.Event
			if err := json.Unmarshal(raw, &event); err != nil {
				return nil, err
			}
			if e.OnEvent != nil {
				return nil, e.OnEvent(callCtx, event)
			}
			return nil, nil
		case "prompt.prepare":
			var input protocol.Prompt
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if e.OnPrompt == nil {
				return nil, errors.New("prompt hook is not registered")
			}
			return e.OnPrompt(callCtx, input)
		case "domain.transform":
			var input protocol.TransformCall
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if e.Transforms == nil {
				return nil, errors.New("transforms are not registered")
			}
			return e.Transforms.apply(callCtx, input)
		case "provider.prepare", "provider.generate":
			var input protocol.ProviderCall
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			for _, provider := range e.Providers {
				if provider.Meta.ID == input.Provider {
					if !input.AuthDisabled && input.Credential.Type == "" && input.Identity.ClientID != "" {
						if err := e.host.CredentialGet(callCtx, input.Provider, &input.Credential); err != nil {
							return nil, err
						}
					}
					if method == "provider.prepare" {
						if provider.Prepare == nil {
							return nil, errors.New("provider has no request preparation")
						}
						return provider.Prepare(callCtx, input)
					}
					return provider.Stream(callCtx, input, func(part models.StreamPart) error {
						if err := callCtx.Err(); err != nil {
							return err
						}
						data, err := models.MarshalStreamPart(part)
						if err != nil {
							return err
						}
						return peer.Notify("provider.stream", protocol.StreamUpdate{RequestID: pluginrpc.RequestID(callCtx), Part: data})
					})
				}
			}
		}
		return nil, fmt.Errorf("unknown plugin operation %q", method)
	})
	go func() { <-ctx.Done(); peer.Close(); _ = reader.Close() }()
	err := peer.Serve(ctx, reader)
	stopWork()
	peer.Close()
	peer.Wait()
	return errors.Join(err, closeExtension(context.Background()))
}

// CredentialGet reads a plugin-owned credential shared across this client's projects.
func (h *Host) CredentialGet(ctx context.Context, providerID string, output *provider.Credential) error {
	return h.Call(ctx, "credential.get", map[string]any{"key": providerID}, output)
}

// CredentialSet stores a credential without returning it to an API client.
func (h *Host) CredentialSet(ctx context.Context, providerID string, credential provider.Credential) error {
	return h.Call(ctx, "credential.set", map[string]any{"key": providerID, "value": credential}, nil)
}

// CredentialRemove removes a plugin-owned credential.
func (h *Host) CredentialRemove(ctx context.Context, providerID string) error {
	return h.Call(ctx, "credential.remove", map[string]any{"key": providerID}, nil)
}

func (e *Extension) contributions() (protocol.Contributions, error) {
	result := protocol.Contributions{Parts: e.Parts, ContextBoundary: e.ContextBoundary}
	result.Transforms = e.Transforms != nil
	for _, auth := range e.Authentication {
		if auth.Handle == nil {
			return result, errors.New("authentication handler is required")
		}
		result.Auth = append(result.Auth, auth.AuthMethod)
	}
	seen := map[string]bool{}
	check := func(kind, name string, valid bool) error {
		key := kind + ":" + name
		if name == "" || seen[key] || !valid {
			return fmt.Errorf("invalid or duplicate %s %q", kind, name)
		}
		seen[key] = true
		return nil
	}
	for _, t := range e.Tools {
		if err := check("tool", t.Name, t.Execute != nil); err != nil {
			return result, err
		}
		result.Tools = append(result.Tools, t.Tool)
	}
	for _, a := range e.Actions {
		if err := check("action", a.ID, a.Execute != nil); err != nil {
			return result, err
		}
		result.Actions = append(result.Actions, a.Action)
	}
	for _, p := range e.Providers {
		if err := check("provider", p.Meta.ID, p.Stream != nil); err != nil {
			return result, err
		}
		result.Providers = append(result.Providers, p.Provider)
	}
	for _, m := range e.Methods {
		if err := check("method", m.Name, m.Execute != nil); err != nil {
			return result, err
		}
		result.Methods = append(result.Methods, m.Method)
	}
	hooks := map[string]bool{"before_run": e.Hooks.BeforeRun != nil, "transform_system": e.Hooks.TransformSystem != nil, "transform_context": e.Hooks.TransformContext != nil, "transform_history": e.Hooks.TransformHistory != nil, "transform_tools": e.Hooks.TransformToolDefs != nil, "transform_params": e.Hooks.TransformParams != nil, "before_tool": e.Hooks.BeforeToolCall != nil, "after_tool": e.Hooks.AfterToolCall != nil, "before_complete": e.Hooks.BeforeComplete != nil, "after_run": e.Hooks.AfterRun != nil}
	for name, enabled := range hooks {
		if enabled {
			result.Hooks = append(result.Hooks, name)
		}
	}
	if e.Hooks.OnTurnStart != nil {
		result.Hooks = append(result.Hooks, "turn_start")
	}
	if e.Hooks.OnTurnEnd != nil {
		result.Hooks = append(result.Hooks, "turn_end")
	}
	if e.OnEvent != nil {
		result.Hooks = append(result.Hooks, "events")
	}
	if e.OnPrompt != nil {
		result.Hooks = append(result.Hooks, "prompt")
	}
	if e.HTTP.Request != nil {
		result.Hooks = append(result.Hooks, "http.request")
	}
	if e.HTTP.Response != nil {
		result.Hooks = append(result.Hooks, "http.response")
	}
	sort.Strings(result.Hooks)
	return result, nil
}
