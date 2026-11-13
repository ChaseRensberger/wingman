package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

// Registration removes a callback from future snapshots.
type Registration struct {
	once    sync.Once
	dispose func()
}

// Dispose removes the registration once; captured runtime values remain usable.
func (r *Registration) Dispose() { r.once.Do(r.dispose) }

type transform struct {
	id     uint64
	domain string
	apply  func(context.Context, json.RawMessage) (json.RawMessage, error)
}

// Transforms owns reversible callbacks replayed in registration order on each fresh snapshot.
// Callbacks must be repeatable and must not retain or concurrently mutate their drafts.
type Transforms struct {
	mu      sync.RWMutex
	next    uint64
	entries []transform
}

func registerTransform[T any](t *Transforms, domain string, fn func(context.Context, T) error) *Registration {
	if fn == nil {
		panic("plugin: nil transform")
	}
	t.mu.Lock()
	t.next++
	id := t.next
	t.entries = append(t.entries, transform{id: id, domain: domain, apply: func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var draft T
		if err := json.Unmarshal(raw, &draft); err != nil {
			return nil, err
		}
		if err := fn(ctx, draft); err != nil {
			return nil, err
		}
		return json.Marshal(draft)
	}})
	t.mu.Unlock()
	return &Registration{dispose: func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for i, entry := range t.entries {
			if entry.id == id {
				t.entries = append(t.entries[:i], t.entries[i+1:]...)
				return
			}
		}
	}}
}

// Agents registers a reversible agent policy.
func (t *Transforms) Agents(fn func(context.Context, protocol.Agents) error) *Registration {
	return registerTransform(t, "agents", fn)
}

// Providers registers a reversible provider-source policy.
func (t *Transforms) Providers(fn func(context.Context, protocol.Providers) error) *Registration {
	return registerTransform(t, "providers", fn)
}

// Models registers a policy over the models remaining after provider transforms.
func (t *Transforms) Models(fn func(context.Context, protocol.Models) error) *Registration {
	return registerTransform(t, "models", fn)
}

// MCP registers a reversible MCP configuration policy.
func (t *Transforms) MCP(fn func(context.Context, protocol.MCPServers) error) *Registration {
	return registerTransform(t, "mcp", fn)
}

// Skills registers a reversible skill policy.
func (t *Transforms) Skills(fn func(context.Context, protocol.Skills) error) *Registration {
	return registerTransform(t, "skills", fn)
}

// References registers a reversible reference policy.
func (t *Transforms) References(fn func(context.Context, protocol.References) error) *Registration {
	return registerTransform(t, "references", fn)
}

func (t *Transforms) apply(ctx context.Context, call protocol.TransformCall) (json.RawMessage, error) {
	t.mu.RLock()
	entries := append([]transform(nil), t.entries...)
	t.mu.RUnlock()
	value := call.Value
	for _, entry := range entries {
		if entry.domain != call.Domain {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var err error
		value, err = entry.apply(ctx, value)
		if err != nil {
			return nil, fmt.Errorf("transform %s: %w", call.Domain, err)
		}
	}
	return value, ctx.Err()
}
