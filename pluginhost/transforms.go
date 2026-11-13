package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

// Transform replays captured plugins against an owned domain draft in plugin order.
func (a *Activation) Transform(ctx context.Context, domain string, input, output any) error {
	active, _ := ctx.Value(transformKey{}).(map[string]bool)
	if active[domain] {
		return fmt.Errorf("recursive %s transform is not supported; inspect the supplied draft", domain)
	}
	next := make(map[string]bool, len(active)+1)
	for key, value := range active {
		next[key] = value
	}
	next[domain] = true
	ctx = context.WithValue(ctx, transformKey{}, next)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	value, err := json.Marshal(input)
	if err != nil {
		return err
	}
	for _, p := range a.plugins {
		if !p.contributions.Transforms {
			continue
		}
		var next json.RawMessage
		if err := a.Call(ctx, p.id.ID, "domain.transform", protocol.TransformCall{Identity: a.identity, Domain: domain, Value: value}, &next); err != nil {
			return fmt.Errorf("plugin %q: %w", p.id.ID, err)
		}
		if string(next) == "null" || !json.Valid(next) {
			return fmt.Errorf("plugin %q returned an invalid %s draft", p.id.ID, domain)
		}
		value = next
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return json.Unmarshal(value, output)
}

type transformKey struct{}

// HasTransforms reports whether this activation contains domain policies.
func (a *Activation) HasTransforms() bool {
	for _, p := range a.plugins {
		if p.contributions.Transforms {
			return true
		}
	}
	return false
}

// ProviderRegistry captures provider sources followed by policies over their model inventory.
func (a *Activation) ProviderRegistry(ctx context.Context, base *provider.Registry) (*provider.Registry, error) {
	registry, err := base.WithRegistrations(a.Providers())
	if err != nil {
		return nil, err
	}
	if !a.HasTransforms() {
		return registry, nil
	}
	sources := protocol.Providers(registry.Sources())
	var providers protocol.Providers
	if err := a.Transform(ctx, "providers", sources, &providers); err != nil {
		return nil, err
	}
	models := protocol.Models{}
	for id, source := range providers {
		models[id] = source.Models
	}
	var transformed protocol.Models
	if err := a.Transform(ctx, "models", models, &transformed); err != nil {
		return nil, err
	}
	for id := range transformed {
		if _, ok := providers[id]; !ok {
			return nil, fmt.Errorf("model transform added unavailable provider %q", id)
		}
	}
	for id, source := range providers {
		source.Models = transformed[id]
		providers[id] = source
	}
	return registry.WithSources(providers)
}
