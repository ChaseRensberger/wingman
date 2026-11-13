package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/store"
)

type pluginEventSubscription struct {
	client   string
	plugin   string
	lifetime context.Context
	types    map[string]bool
	events   chan protocol.Event
	stop     func() bool
	done     chan struct{}
	overflow bool
	idle     *time.Timer
}

type pluginEventBus struct {
	mu     sync.Mutex
	subs   map[string]*pluginEventSubscription
	closed bool
}

func (b *pluginEventBus) shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for id, sub := range b.subs {
		delete(b.subs, id)
		if !sub.overflow {
			close(sub.done)
		}
		if sub.stop != nil {
			sub.stop()
		}
		if sub.idle != nil {
			sub.idle.Stop()
		}
	}
}

type pluginEventRequest struct {
	Operation string   `json:"operation" enum:"subscribe,next,close"`
	ID        string   `json:"id,omitempty"`
	Types     []string `json:"types,omitempty"`
}
type pluginEventResponse struct {
	ID     string           `json:"id,omitempty"`
	Events []protocol.Event `json:"events,omitempty"`
}

func (b *pluginEventBus) publish(event protocol.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		if sub.client != event.Identity.ClientID || sub.overflow || (len(sub.types) > 0 && !sub.types[event.Type]) {
			continue
		}
		if len(event.Data) <= 64*1024 {
			select {
			case sub.events <- event:
				continue
			default:
			}
		}
		sub.overflow = true
		close(sub.done)
	}
}

func (b *pluginEventBus) close(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub := b.subs[id]; sub != nil {
		delete(b.subs, id)
		if !sub.overflow {
			close(sub.done)
		}
		if sub.stop != nil {
			sub.stop()
		}
		if sub.idle != nil {
			sub.idle.Stop()
		}
	}
}

func (b *pluginEventBus) call(ctx, lifetime context.Context, client, plugin, operation string, raw json.RawMessage) (any, error) {
	if lifetime == nil || client == "" {
		return nil, errors.New("event subscription requires a client-owned lifetime")
	}
	if err := lifetime.Err(); err != nil {
		return nil, err
	}
	var input pluginEventRequest
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if operation == "events.subscribe" {
		id := store.NewID("sub")
		sub := &pluginEventSubscription{client: client, plugin: plugin, lifetime: lifetime, types: map[string]bool{}, events: make(chan protocol.Event, 128), done: make(chan struct{})}
		for _, typ := range input.Types {
			sub.types[typ] = true
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return nil, errors.New("event bus is closed")
		}
		if len(b.subs) >= 256 {
			b.mu.Unlock()
			return nil, errors.New("too many event subscriptions")
		}
		b.subs[id] = sub
		sub.stop = context.AfterFunc(lifetime, func() { b.close(id) })
		if plugin == "" {
			sub.idle = time.AfterFunc(time.Minute, func() { b.close(id) })
		}
		b.mu.Unlock()
		return id, nil
	}
	b.mu.Lock()
	sub := b.subs[input.ID]
	if sub == nil || sub.client != client || sub.plugin != plugin || sub.lifetime != lifetime {
		b.mu.Unlock()
		return nil, errors.New("event subscription not found")
	}
	overflow := sub.overflow
	if sub.idle != nil {
		sub.idle.Reset(time.Minute)
	}
	b.mu.Unlock()
	if operation == "events.close" {
		b.close(input.ID)
		return nil, nil
	}
	if operation != "events.next" {
		return nil, errors.New("invalid event operation")
	}
	if overflow {
		b.close(input.ID)
		return nil, errors.New("event subscription overflow; resubscribe and refresh state")
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-sub.done:
		b.close(input.ID)
		return nil, errors.New("event subscription closed or overflowed")
	case <-timer.C:
		return []protocol.Event{}, nil
	case event := <-sub.events:
		result := []protocol.Event{event}
		for len(result) < 32 {
			select {
			case event := <-sub.events:
				result = append(result, event)
			default:
				return result, nil
			}
		}
		return result, nil
	}
}

func (s *Server) pluginEventCall(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	identity, owner := pluginhost.Identity(ctx), pluginhost.PluginID(ctx)
	if method == "events.publish" {
		var input struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if identity.ClientID == "" || owner == "" || strings.TrimSpace(input.Type) == "" || len(input.Type) > 200 || len(input.Data) > 64*1024 || !json.Valid(input.Data) {
			return nil, errors.New("invalid plugin event")
		}
		s.pluginEvents.publish(protocol.Event{Identity: identity, Type: "plugin." + owner + "." + input.Type, Data: input.Data})
		return nil, nil
	}
	return s.pluginEvents.call(ctx, pluginhost.Lifetime(ctx), identity.ClientID, owner, method, raw)
}

func (s *Server) handlePluginEvents(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	client, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input pluginEventRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid event request")
		return
	}
	raw, _ := json.Marshal(input)
	result, err := s.pluginEvents.call(r.Context(), s.shutdownCtx, client, "", "events."+input.Operation, raw)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response := pluginEventResponse{}
	switch value := result.(type) {
	case string:
		response.ID = value
	case []protocol.Event:
		response.Events = value
	}
	writeJSON(w, http.StatusOK, response)
}
