package pluginhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/internal/pluginrpc"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

type streamKey struct{}
type identityKey struct{}
type pluginKey struct{}
type modelClientKey struct{}
type eventSinkKey struct{}
type pluginLifetimeKey struct{}
type scopeGrantKey struct{}

type scopeGrant struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Lifetime returns the executable lifetime that owns a host callback.
func Lifetime(ctx context.Context) context.Context {
	value, _ := ctx.Value(pluginLifetimeKey{}).(context.Context)
	return value
}

// PluginID returns the identity of the plugin making a host call.
func PluginID(ctx context.Context) string { id, _ := ctx.Value(pluginKey{}).(string); return id }

// WithIdentity associates host callbacks with the execution that owns them.
func WithIdentity(ctx context.Context, identity protocol.Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// Identity returns the host-validated execution identity.
func Identity(ctx context.Context) protocol.Identity {
	identity, _ := ctx.Value(identityKey{}).(protocol.Identity)
	return identity
}

func (c *rpcClient) dispatchHostCall(envelope rpcEnvelope) {
	ctx, cancel := context.WithCancel(c.processCtx)
	id := string(envelope.ID)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel()
		return
	}
	if _, exists := c.hostCalls[id]; exists {
		c.mu.Unlock()
		cancel()
		c.terminate(errors.New("duplicate active host request"))
		return
	}
	c.hostCalls[id] = cancel
	c.mu.Unlock()
	go func() {
		defer cancel()
		defer func() {
			c.mu.Lock()
			delete(c.hostCalls, id)
			c.mu.Unlock()
		}()
		c.handleHostCall(ctx, envelope)
	}()
}

func (c *rpcClient) handleHostCall(requestCtx context.Context, envelope rpcEnvelope) {
	var input protocol.HostCall
	err := json.Unmarshal(envelope.Params, &input)
	c.mu.Lock()
	pending := c.pending[input.InvocationID]
	handler := c.hostHandler
	bound := c.bindings[input.ScopeToken]
	c.mu.Unlock()
	var result any
	if err == nil {
		var owner context.Context
		if pending != nil {
			if pending.beginCallback() {
				defer pending.callbacks.Done()
				owner = pending.ctx
			}
		} else {
			owner = bound
		}
		if owner == nil || handler == nil {
			err = errors.New("host call requires an active invocation")
		} else if owner.Err() != nil {
			err = owner.Err()
		} else if requestCtx.Err() != nil {
			err = requestCtx.Err()
		} else if bound != nil && bound.Value(httpBodyGrantKey{}) != nil && input.Method != "http.body.read" && input.Method != "scope.release" {
			err = errors.New("HTTP body grant only permits source reads")
		} else if input.Method == "scope.release" {
			c.mu.Lock()
			delete(c.bindings, input.ScopeToken)
			if bound != nil {
				if grant, _ := bound.Value(httpBodyGrantKey{}).(*httpBodyGrant); grant != nil {
					grant.stop()
				}
				if grant, _ := bound.Value(scopeGrantKey{}).(*scopeGrant); grant != nil {
					grant.cancel()
				}
			}
			c.mu.Unlock()
		} else if input.Method == "scope.bind" || input.Method == "http.body.bind" {
			var bytes [32]byte
			if _, err = rand.Read(bytes[:]); err == nil {
				token := hex.EncodeToString(bytes[:])
				c.mu.Lock()
				if input.Method == "http.body.bind" {
					lifetime, _ := owner.Value(httpBodyLifetimeKey{}).(context.Context)
					if lifetime == nil {
						err = errors.New("HTTP source is unavailable outside an exchange")
					} else {
						grant := &httpBodyGrant{}
						grant.stop = context.AfterFunc(lifetime, func() { c.mu.Lock(); delete(c.bindings, token); c.mu.Unlock() })
						c.bindings[token] = WithIdentity(context.WithValue(lifetime, httpBodyGrantKey{}, grant), Identity(owner))
					}
				} else {
					ctx, cancel := context.WithCancel(c.processCtx)
					grant := &scopeGrant{ctx: ctx, cancel: cancel}
					c.bindings[token] = WithIdentity(context.WithValue(ctx, scopeGrantKey{}, grant), Identity(owner))
				}
				c.mu.Unlock()
				result = token
			}
		} else {
			if pending != nil && input.Method == "run.emit" {
				pending.emitMu.Lock()
				defer pending.emitMu.Unlock()
			}
			lifetime := c.processCtx
			if grant, _ := owner.Value(scopeGrantKey{}).(*scopeGrant); grant != nil {
				lifetime = grant.ctx
			}
			ctx := context.WithValue(owner, pluginLifetimeKey{}, lifetime)
			var cancel context.CancelFunc
			if input.Method == "http.body.read" {
				ctx, cancel = context.WithCancel(ctx)
			} else {
				ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			}
			stop := context.AfterFunc(requestCtx, cancel)
			if requestCtx.Err() != nil {
				cancel()
			}
			if err = ctx.Err(); err == nil {
				result, err = handler(ctx, input.Method, input.Input)
			}
			stop()
			cancel()
		}
	}
	data, marshalErr := json.Marshal(result)
	if err == nil {
		err = marshalErr
	}
	response := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *rpcError       `json:"error,omitempty"`
	}{JSONRPC: "2.0", ID: envelope.ID, Result: data}
	if err != nil {
		response.Result = nil
		response.Error = pluginrpc.NewError(err)
	}
	_ = c.write(response)
}

// SetHostHandler configures the callbacks available to current and future generations.
func (m *Manager) SetHostHandler(handler func(context.Context, string, json.RawMessage) (any, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostHandler = handler
	if m.generation != nil {
		for _, p := range m.generation.plugins {
			p.client.mu.Lock()
			p.client.hostHandler = bindHost(p.id.ID, handler)
			p.client.mu.Unlock()
		}
	}
}

func bindHost(id string, handler func(context.Context, string, json.RawMessage) (any, error)) func(context.Context, string, json.RawMessage) (any, error) {
	if handler == nil {
		return nil
	}
	return func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "http.body.read" {
			return readHTTPBody(ctx)
		}
		if method == "plugin.call" {
			activation, _ := ctx.Value(activationKey{}).(*Activation)
			if activation == nil {
				return nil, fmt.Errorf("plugin calls require an activation")
			}
			var request struct {
				Plugin string          `json:"plugin"`
				Method string          `json:"method"`
				Input  json.RawMessage `json:"input"`
			}
			if err := json.Unmarshal(input, &request); err != nil {
				return nil, err
			}
			var result json.RawMessage
			err := activation.Invoke(ctx, request.Plugin, request.Method, request.Input, &result)
			return result, err
		}
		if method == "model.generate" || method == "model.prepare" {
			client, _ := ctx.Value(modelClientKey{}).(models.Client)
			if client == nil {
				return nil, fmt.Errorf("model callbacks are unavailable in this hook")
			}
			var request models.Request
			if err := json.Unmarshal(input, &request); err != nil {
				return nil, err
			}
			if method == "model.generate" {
				return client.Generate(ctx, request)
			}
			return client.Prepare(ctx, request)
		}
		if method == "run.emit" {
			sink, _ := ctx.Value(eventSinkKey{}).(run.Sink)
			if sink == nil {
				return nil, fmt.Errorf("history events are unavailable in this hook")
			}
			var event run.MessageEvent
			if err := json.Unmarshal(input, &event); err != nil {
				return nil, err
			}
			if event.Message.Role == "" {
				return nil, fmt.Errorf("history events must contain a message")
			}
			sink.OnEvent(event)
			return nil, nil
		}
		return handler(context.WithValue(ctx, pluginKey{}, id), method, input)
	}
}
