package provider

import (
	"fmt"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
)

// Registration supplies an independently implemented model provider.
type Registration struct {
	Meta      ProviderMeta
	Models    map[string]models.ModelInfo
	NewClient func(Credential, ProviderOptions) models.Client
}

// WithRegistrations builds a new generation without changing existing clients.
func (r *Registry) WithRegistrations(registrations []Registration) (*Registry, error) {
	if len(registrations) == 0 {
		return r, nil
	}
	next := &Registry{providers: map[string]ProviderMeta{}, configs: map[string]ProviderConfig{}, routes: r.routes, clients: map[string]func(Credential, ProviderOptions) models.Client{}}
	overlays := map[string]catalog.ProviderOverlay{}
	for id, meta := range r.providers {
		next.providers[id] = cloneMeta(meta)
	}
	for id, cfg := range r.configs {
		next.configs[id] = cloneConfig(cfg)
		overlays[id] = catalog.ProviderOverlay{BaseURL: cfg.Options.BaseURL, Models: cfg.Models}
	}
	for id, factory := range r.clients {
		next.clients[id] = factory
	}
	for _, registration := range registrations {
		id := registration.Meta.ID
		if id == "" || registration.NewClient == nil {
			return nil, fmt.Errorf("invalid provider registration %q", id)
		}
		if _, exists := next.clients[id]; exists {
			return nil, fmt.Errorf("duplicate plugin provider %q", id)
		}
		modelsByID := map[string]models.ModelInfo{}
		for key, info := range registration.Models {
			if info.Provider == "" {
				info.Provider = id
			}
			if info.ID == "" {
				info.ID = key
			}
			if info.API == "" {
				info.API = "plugin"
			}
			if info.BaseURL == "" {
				info.BaseURL = "plugin://" + id
			}
			modelsByID[key] = info
		}
		next.providers[id] = cloneMeta(registration.Meta)
		next.clients[id] = registration.NewClient
		cfg := next.configs[id]
		if cfg.Name == "" {
			cfg.Name = registration.Meta.Name
		}
		cfg.Models = modelsByID
		cfg.AuthTypes = registration.Meta.AuthTypes
		next.configs[id] = cloneConfig(cfg)
		overlays[id] = catalog.ProviderOverlay{Models: modelsByID}
	}
	var err error
	next.catalog, err = catalog.New(overlays)
	return next, err
}
