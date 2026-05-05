// Package catalog provides static metadata about LLM providers and models,
// sourced from https://models.dev/api.json.
//
// Layout:
//   - snapshot.json: a build-time snapshot of the entire models.dev catalog,
//     embedded into the binary so first boot works offline.
//   - Client: holds an in-memory ModelsDB (initialized from the snapshot) and
//     refreshes it from the live API on a background interval.
//   - Get(provider, model) returns a normalized models.ModelInfo for use
//     by Model implementations.
//
// The raw types (ProviderData, Model, ModelCost, ModelLimit, Modalities)
// remain exported for callers that want richer metadata than ModelInfo
// provides (cost data, modalities, knowledge cutoff dates, etc.).
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/models"
)

const (
	modelsDevURL = "https://models.dev/api.json"
	// fetchTimeout caps a single live-fetch attempt. The background refresher
	// will retry on the next tick if the network is slow or down.
	fetchTimeout = 30 * time.Second
	// defaultRefreshInterval is how often the background goroutine re-pulls
	// the live catalog. Hourly is plenty: model metadata changes infrequently.
	defaultRefreshInterval = 1 * time.Hour
)

//go:embed snapshot.json
var snapshotJSON []byte

// Client wraps the ModelsDB with a refresher. Most callers should use the
// package-level functions (Get, GetAll, etc.) which delegate to a default
// Client started at init. Construct your own Client only if you need
// independent refresh schedules or HTTP clients (e.g. tests).
type Client struct {
	httpClient *http.Client

	mu       sync.RWMutex
	cache    ModelsDB
	cachedAt time.Time
	source   string // "snapshot" or "live"
}

// NewClient builds a Client preloaded from the embedded snapshot. The
// snapshot decode is deterministic and offline; failures are programmer
// errors (corrupt embed) and panic.
func NewClient() *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: fetchTimeout},
	}
	var db ModelsDB
	if err := json.Unmarshal(snapshotJSON, &db); err != nil {
		panic(fmt.Errorf("catalog: corrupt embedded snapshot.json: %w", err))
	}
	c.cache = db
	c.cachedAt = time.Time{} // zero indicates snapshot, not live
	c.source = "snapshot"
	return c
}

// StartRefresher starts a background goroutine that refreshes the live
// catalog every interval. Returns immediately; safe to call once. Cancel by
// closing stop.
//
// Refresh failures are intentionally silent: the snapshot remains usable and
// the next tick will retry. We don't log because the catalog package has no
// logger dependency; callers who care can wrap and observe via Source().
func (c *Client) StartRefresher(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	go func() {
		// Kick off an immediate refresh so the first hour isn't snapshot-only.
		_, _ = c.Refresh()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				_, _ = c.Refresh()
			}
		}
	}()
}

// Source reports whether the in-memory catalog is from the embedded snapshot
// ("snapshot") or from a live fetch ("live"). Useful for diagnostics.
func (c *Client) Source() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.source
}

// CachedAt reports when the live catalog was last fetched. Zero if the
// in-memory copy is still the embedded snapshot.
func (c *Client) CachedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedAt
}

// GetAll returns the in-memory catalog. Safe to call without StartRefresher;
// returns the snapshot in that case.
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
// Mapping notes:
//   - Provider/ID copy directly.
//   - ContextWindow comes from Limit.Context.
//   - MaxOutput comes from Limit.Output.
//   - Capabilities.Tools comes from ToolCall.
//   - Capabilities.Images is true if "image" appears in input modalities.
//   - Capabilities.Reasoning comes from Reasoning.
//   - Capabilities.StructuredOutput comes from StructuredOutput.
//   - Cost fields come from Cost.{Input,Output,CacheRead,CacheWrite}; these
//     are already in USD per 1M tokens in the models.dev schema.
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

// Refresh pulls the live catalog. Updates the in-memory copy on success.
// Returns the new ModelsDB and an error; on error the existing in-memory
// copy is unchanged.
func (c *Client) Refresh() (ModelsDB, error) {
	resp, err := c.httpClient.Get(modelsDevURL)
	if err != nil {
		return nil, fmt.Errorf("fetch models.dev: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read models.dev response: %w", err)
	}

	var db ModelsDB
	if err := json.Unmarshal(body, &db); err != nil {
		return nil, fmt.Errorf("parse models.dev response: %w", err)
	}

	c.mu.Lock()
	c.cache = db
	c.cachedAt = time.Now()
	c.source = "live"
	c.mu.Unlock()

	return db, nil
}

// defaultClient is the package-level Client used by Get / GetAll / etc. It is
// preloaded from the snapshot at init and does not auto-refresh; the binary
// entrypoint (cmd/wingman) is responsible for calling StartRefresher if it
// wants live updates. This keeps imports of catalog in tools like example
// programs from spawning background goroutines.
var defaultClient = NewClient()

// Default returns the package-level Client. Use this to call StartRefresher
// from your main.
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
