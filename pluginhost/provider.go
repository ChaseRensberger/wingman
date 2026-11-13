package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

// ProviderRegistrations returns current provider metadata for discovery APIs.
// Executing a run must use Acquire and Activation.Providers instead.
func (m *Manager) ProviderRegistrations() []provider.Registration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.generation == nil {
		return nil
	}
	ids := make([]string, 0, len(m.generation.plugins))
	for id := range m.generation.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	a := &Activation{}
	for _, id := range ids {
		a.plugins = append(a.plugins, m.generation.plugins[id])
	}
	return a.Providers()
}

// Providers returns provider implementations pinned to this activation.
func (a *Activation) Providers() []provider.Registration {
	var registrations []provider.Registration
	for _, p := range a.plugins {
		for _, declaration := range p.contributions.Providers {
			registrations = append(registrations, provider.Registration{Meta: declaration.Meta, Models: declaration.Models, NewClient: func(credential provider.Credential, options provider.ProviderOptions) models.Client {
				return &externalModelClient{activation: a, pluginID: p.id.ID, providerID: declaration.Meta.ID, credential: credential, options: options, authDisabled: options.Auth != nil && !*options.Auth}
			}})
		}
	}
	return registrations
}

type externalModelClient struct {
	activation   *Activation
	pluginID     string
	providerID   string
	credential   provider.Credential
	options      provider.ProviderOptions
	authDisabled bool
}

func (c *externalModelClient) Prepare(ctx context.Context, request models.Request) (*models.PreparedRequest, error) {
	var output models.PreparedRequest
	err := c.activation.Call(ctx, c.pluginID, "provider.prepare", protocol.ProviderCall{Identity: c.activation.identity, Provider: c.providerID, Request: request, Credential: c.credential, Options: c.options, AuthDisabled: c.authDisabled}, &output)
	return &output, err
}

func (c *externalModelClient) Stream(ctx context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	ctx, cancel := context.WithCancel(ctx)
	stream := models.NewEventStream[models.StreamPart, *models.Message](32)
	stream.BindContext(ctx)
	updates := make(chan json.RawMessage, 128)
	var queuedBytes atomic.Int64
	type response struct {
		message models.Message
		err     error
	}
	completed := make(chan response, 1)
	callCtx := context.WithValue(ctx, streamKey{}, func(raw json.RawMessage) error {
		if queuedBytes.Add(int64(len(raw))) > 8<<20 {
			queuedBytes.Add(-int64(len(raw)))
			return fmt.Errorf("plugin provider stream consumer is too slow")
		}
		select {
		case updates <- raw:
			return nil
		default:
			queuedBytes.Add(-int64(len(raw)))
			return fmt.Errorf("plugin provider stream consumer is too slow")
		}
	})
	go func() {
		var result response
		result.err = c.activation.Call(callCtx, c.pluginID, "provider.generate", protocol.ProviderCall{Identity: c.activation.identity, Provider: c.providerID, Request: request, Credential: c.credential, Options: c.options, AuthDisabled: c.authDisabled}, &result.message)
		completed <- result
	}()
	var first json.RawMessage
	select {
	case first = <-updates:
	case result := <-completed:
		// Only failures before any notification are safe startup rejections.
		if result.err != nil && len(updates) == 0 {
			cancel()
			return nil, result.err
		}
		completed <- result
	}
	go func() {
		defer cancel()
		var deliveryErr error
		deliver := func(raw json.RawMessage) {
			queuedBytes.Add(-int64(len(raw)))
			if deliveryErr != nil {
				return
			}
			part, err := models.UnmarshalStreamPart(raw)
			if err != nil {
				deliveryErr = fmt.Errorf("invalid plugin provider stream: %w", err)
				cancel()
				return
			}
			stream.Push(part)
		}
		if first != nil {
			deliver(first)
		}
		for {
			select {
			case raw := <-updates:
				deliver(raw)
			case result := <-completed:
				for len(updates) > 0 {
					deliver(<-updates)
				}
				if deliveryErr != nil {
					result.err = deliveryErr
				}
				if result.err == nil && result.message.Role != models.RoleAssistant {
					result.err = fmt.Errorf("plugin provider returned a non-assistant message")
				}
				stream.Close(&result.message, result.err)
				return
			}
		}
	}()
	return stream, nil
}

func (c *externalModelClient) Generate(ctx context.Context, request models.Request) (*models.Message, error) {
	return models.Generate(ctx, c, request)
}
