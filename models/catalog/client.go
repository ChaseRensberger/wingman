// Package catalog provides bundled metadata about LLM providers and models.
//
// Layout:
//   - data/: TOML source for the catalog.
//   - wingmodels_snapshot.json: a generated snapshot embedded into the binary.
//   - Client: holds an in-memory ModelsDB projected from the snapshot.
//   - Get(provider, model) returns a normalized models.ModelInfo for use
//     by Model implementations.
//
// The raw types (ProviderData, Model, ModelCost, ModelLimit, Modalities)
// remain exported for callers that want richer metadata than ModelInfo provides.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/chaserensberger/wingman/models"
)

//go:embed wingmodels_snapshot.json
var snapshotJSON []byte

// Client wraps the projected model catalog. Most callers should use the
// package-level functions (Get, GetAll, etc.) which delegate to a default Client.
type Client struct {
	mu    sync.RWMutex
	cache ModelsDB
}

// NewClient builds a Client preloaded from the embedded snapshot.
func NewClient() *Client {
	c := &Client{}
	var snapshot Snapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		panic(fmt.Errorf("catalog: corrupt embedded wingmodels_snapshot.json: %w", err))
	}
	c.cache = projectSnapshot(snapshot)
	return c
}

// GetAll returns the in-memory catalog.
func (c *Client) GetAll() ModelsDB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache
}

// GetProvider returns the provider entry by id (e.g. "anthropic"). Returns
// (nil, false) if absent.
func (c *Client) GetProvider(name string) (*ProviderData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.cache[name]
	if !ok {
		return nil, false
	}
	return &p, true
}

// GetModel returns the raw catalog entry for a given provider/model. Returns
// (nil, false) if either is absent.
func (c *Client) GetModel(providerName, modelID string) (*Model, bool) {
	p, ok := c.GetProvider(providerName)
	if !ok {
		return nil, false
	}
	m, ok := p.Models[modelID]
	if !ok {
		return nil, false
	}
	return &m, true
}

// GetModels returns the model map for a provider. Returns (nil, false) if the
// provider is unknown.
func (c *Client) GetModels(providerName string) (map[string]Model, bool) {
	p, ok := c.GetProvider(providerName)
	if !ok {
		return nil, false
	}
	return p.Models, true
}

// Get returns a normalized models.ModelInfo for a provider/model pair.
// This is the adapter Model implementations call from their Info() method.
//
// API, BaseURL and Compat are NOT populated here: those are
// provider-implementation concerns set by the provider's factory after
// calling Get.
func (c *Client) Get(providerName, modelID string) (models.ModelInfo, bool) {
	m, ok := c.GetModel(providerName, modelID)
	if !ok {
		return models.ModelInfo{}, false
	}
	return models.ModelInfo{
		Provider:      providerName,
		ID:            modelID,
		ContextWindow: m.Limit.Context,
		MaxOutput:     m.Limit.Output,
		Capabilities: models.ModelCapabilities{
			Tools:            m.ToolCall,
			Images:           slices.Contains(m.Modalities.Input, "image"),
			Reasoning:        m.Reasoning,
			StructuredOutput: m.StructuredOutput,
		},
		InputCostPerMTok:      m.Cost.Input,
		OutputCostPerMTok:     m.Cost.Output,
		CacheReadCostPerMTok:  m.Cost.CacheRead,
		CacheWriteCostPerMTok: m.Cost.CacheWrite,
	}, true
}

// defaultClient is the package-level Client used by Get / GetAll / etc.
var defaultClient = NewClient()

// Default returns the package-level Client.
func Default() *Client { return defaultClient }

// Get is the package-level convenience wrapper around defaultClient.Get.
func Get(providerName, modelID string) (models.ModelInfo, bool) {
	return defaultClient.Get(providerName, modelID)
}

// GetAll, GetProvider, GetModel mirror Client methods for convenience.
func GetAll() ModelsDB { return defaultClient.GetAll() }
func GetProvider(name string) (*ProviderData, bool) {
	return defaultClient.GetProvider(name)
}
func GetModel(providerName, modelID string) (*Model, bool) {
	return defaultClient.GetModel(providerName, modelID)
}
func GetModels(providerName string) (map[string]Model, bool) {
	return defaultClient.GetModels(providerName)
}

func projectSnapshot(snapshot Snapshot) ModelsDB {
	providers := make(map[string]Provider, len(snapshot.Providers))
	for _, provider := range snapshot.Providers {
		providers[provider.ID] = provider
	}

	labModels := make(map[string]LabModel, len(snapshot.LabModels))
	for _, model := range snapshot.LabModels {
		labModels[model.ID] = model
	}

	db := make(ModelsDB, len(snapshot.Providers))
	for _, provider := range snapshot.Providers {
		data := ProviderData{
			ID:     provider.ID,
			Name:   provider.DisplayName,
			Models: make(map[string]Model),
		}
		if provider.Auth != nil {
			data.Env = append([]string(nil), provider.Auth.EnvVars...)
		}
		if len(provider.BaseURLs) > 0 {
			data.API = provider.BaseURLs[0]
		}
		db[provider.ID] = data
	}

	for _, providerModel := range snapshot.ProviderModels {
		providerID, modelID, ok := splitProviderModelID(providerModel.ID)
		if !ok {
			continue
		}
		provider, ok := providers[providerID]
		if !ok {
			continue
		}

		providerData := db[providerID]
		labModel := labModels[modelID]
		providerData.Models[modelID] = projectModel(providerModel, labModel)
		providerData.Name = provider.DisplayName
		db[providerID] = providerData
	}

	return db
}

func splitProviderModelID(id string) (string, string, bool) {
	providerID, modelID, ok := strings.Cut(id, "/")
	return providerID, modelID, ok && providerID != "" && modelID != ""
}

func projectModel(providerModel ProviderModel, labModel LabModel) Model {
	modelID := providerModel.ID
	if _, id, ok := splitProviderModelID(providerModel.ID); ok {
		modelID = id
	}

	modalities := Modalities{}
	if providerModel.Modalities != nil {
		modalities = *providerModel.Modalities
	} else if labModel.Modalities != nil {
		modalities = *labModel.Modalities
	}

	model := Model{
		ID:               modelID,
		Name:             providerModel.DisplayName,
		Family:           labModel.Family,
		Reasoning:        hasCapability(providerModel, CapabilityReasoning),
		ToolCall:         hasCapability(providerModel, CapabilityToolCalling) || hasCapability(providerModel, CapabilityFunctionCalling),
		StructuredOutput: hasCapability(providerModel, CapabilityStructuredOutput),
		Temperature:      slices.Contains(providerModel.SupportedParameters, ParameterTemperature),
		Knowledge:        labModel.KnowledgeCutoff,
		ReleaseDate:      labModel.ReleaseDate,
		OpenWeights:      labModel.OpenWeights,
		Modalities:       modalities,
	}

	if providerModel.Pricing != nil {
		model.Cost = ModelCost{
			Input:      providerModel.Pricing.InputPerMillion,
			Output:     providerModel.Pricing.OutputPerMillion,
			CacheRead:  providerModel.Pricing.CacheReadPerMillion,
			CacheWrite: providerModel.Pricing.CacheWritePerMillion,
		}
	}
	if providerModel.Limits != nil {
		model.Limit = ModelLimit{
			Context: providerModel.Limits.ContextWindow,
			Input:   providerModel.Limits.MaxInputTokens,
			Output:  providerModel.Limits.MaxOutputTokens,
		}
	}
	return model
}

func hasCapability(providerModel ProviderModel, capabilityID CapabilityID) bool {
	for _, capability := range providerModel.Capabilities {
		if capability.ID == capabilityID {
			return true
		}
	}
	return false
}
