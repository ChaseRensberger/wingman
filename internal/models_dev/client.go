package models_dev

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	modelsDevURL = "https://models.dev/api.json"
	cacheTTL     = 1 * time.Hour
	fetchTimeout = 30 * time.Second
)

type Client struct {
	httpClient *http.Client
	cache      ModelsDB
	cachedAt   time.Time
	mu         sync.RWMutex
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: fetchTimeout},
	}
}

func (c *Client) GetAll() (ModelsDB, error) {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cachedAt) < cacheTTL {
		cached := c.cache
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	return c.refresh()
}

func (c *Client) GetProvider(name string) (*ProviderData, error) {
	db, err := c.GetAll()
	if err != nil {
		return nil, err
	}

	provider, ok := db[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}

	return &provider, nil
}

func (c *Client) GetModels(providerName string) (map[string]Model, error) {
	provider, err := c.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	return provider.Models, nil
}

func (c *Client) GetModel(providerName, modelID string) (*Model, error) {
	models, err := c.GetModels(providerName)
	if err != nil {
		return nil, err
	}

	model, ok := models[modelID]
	if !ok {
		return nil, fmt.Errorf("model not found: %s/%s", providerName, modelID)
	}

	return &model, nil
}

func (c *Client) Refresh() (ModelsDB, error) {
	return c.refresh()
}

func (c *Client) refresh() (ModelsDB, error) {
	resp, err := c.httpClient.Get(modelsDevURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models.dev: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read models.dev response: %w", err)
	}

	var db ModelsDB
	if err := json.Unmarshal(body, &db); err != nil {
		return nil, fmt.Errorf("failed to parse models.dev response: %w", err)
	}

	c.mu.Lock()
	c.cache = db
	c.cachedAt = time.Now()
	c.mu.Unlock()

	return db, nil
}

var defaultClient = NewClient()

func GetAll() (ModelsDB, error) {
	return defaultClient.GetAll()
}

func GetProvider(name string) (*ProviderData, error) {
	return defaultClient.GetProvider(name)
}

func GetModels(providerName string) (map[string]Model, error) {
	return defaultClient.GetModels(providerName)
}

func GetModel(providerName, modelID string) (*Model, error) {
	return defaultClient.GetModel(providerName, modelID)
}

func Refresh() (ModelsDB, error) {
	return defaultClient.Refresh()
}
