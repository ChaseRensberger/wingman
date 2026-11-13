package provider

import (
	"fmt"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/route"
)

// Source is a provider's editable metadata, route options, and full model inventory.
type Source struct {
	Meta    ProviderMeta                `json:"meta"`
	Options ProviderOptions             `json:"options"`
	Models  map[string]models.ModelInfo `json:"models"`
}

// Sources returns owned definitions, including built-in and executable providers.
func (r *Registry) Sources() map[string]Source {
	out := map[string]Source{}
	for id, meta := range r.providers {
		inventory, _ := r.catalog.GetModels(id)
		if inventory == nil {
			inventory = map[string]models.ModelInfo{}
		}
		out[id] = Source{Meta: cloneMeta(meta), Options: cloneOptions(r.configs[id].Options), Models: inventory}
	}
	return out
}

// WithSources replaces the inventory without changing previously captured registries or clients.
func (r *Registry) WithSources(sources map[string]Source) (*Registry, error) {
	next := &Registry{providers: map[string]ProviderMeta{}, configs: map[string]ProviderConfig{}, routes: map[string]func(RouteConfig) (route.Route, error){}, clients: map[string]func(Credential, ProviderOptions) models.Client{}}
	inventory := map[string]catalog.ProviderOverlay{}
	for id, source := range sources {
		if id == "" || source.Meta.ID != id {
			return nil, fmt.Errorf("provider %q identity does not match its key", id)
		}
		for key, model := range source.Models {
			if key == "" || model.ID != key || model.Provider != id {
				return nil, fmt.Errorf("invalid model identity %s/%s", id, key)
			}
			if r.clients[id] == nil {
				if _, err := protocolFor(model.API); err != nil {
					return nil, err
				}
			}
		}
		next.providers[id] = cloneMeta(source.Meta)
		next.configs[id] = cloneConfig(ProviderConfig{Name: source.Meta.Name, AuthTypes: source.Meta.AuthTypes, Options: source.Options, Models: source.Models})
		if factory := r.routes[id]; factory != nil {
			next.routes[id] = factory
		}
		if factory := r.clients[id]; factory != nil {
			next.clients[id] = factory
		}
		inventory[id] = catalog.ProviderOverlay{Name: source.Meta.Name, BaseURL: source.Meta.BaseURL, Models: source.Models}
	}
	var err error
	next.catalog, err = r.catalog.WithInventory(inventory)
	return next, err
}
