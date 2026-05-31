// Package catalog loads the intentionally-small built-in model catalog.
package catalog

import (
	"embed"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/chaserensberger/wingman/models"
)

//go:embed providers/*/provider.toml providers/*/models/*.toml
var fs embed.FS

type modelFile struct {
	ID            string   `toml:"id"`
	Provider      string   `toml:"provider"`
	API           string   `toml:"api"`
	BaseURL       string   `toml:"base_url"`
	Env           []string `toml:"env"`
	ContextWindow int      `toml:"context_window"`
	MaxOutput     int      `toml:"max_output"`
	Capabilities  struct {
		Tools            bool `toml:"tools"`
		Images           bool `toml:"images"`
		Reasoning        bool `toml:"reasoning"`
		StructuredOutput bool `toml:"structured_output"`
	} `toml:"capabilities"`
}

type providerFile struct {
	BaseURL string   `toml:"base_url"`
	Env     []string `toml:"env"`
}

var (
	loadOnce       sync.Once
	loadErr        error
	byRef          map[string]models.ModelInfo
	byProv         map[string]map[string]models.ModelInfo
	byDefault      map[string]providerFile
	overlayMu      sync.RWMutex
	overlayRef     = map[string]models.ModelInfo{}
	overlayProv    = map[string]map[string]models.ModelInfo{}
	overlayDefault = map[string]providerFile{}
)

func load() error {
	loadOnce.Do(func() {
		byRef = map[string]models.ModelInfo{}
		byProv = map[string]map[string]models.ModelInfo{}
		byDefault = map[string]providerFile{}
		entries, err := fs.ReadDir("providers")
		if err != nil {
			loadErr = err
			return
		}
		for _, providerDir := range entries {
			if !providerDir.IsDir() {
				continue
			}
			provider := providerDir.Name()
			providerDefaults, err := readProviderFile(provider)
			if err != nil {
				loadErr = err
				return
			}
			byDefault[provider] = providerDefaults
			files, err := fs.ReadDir(filepath.Join("providers", provider, "models"))
			if err != nil {
				loadErr = err
				return
			}
			for _, file := range files {
				if file.IsDir() || !strings.HasSuffix(file.Name(), ".toml") {
					continue
				}
				path := filepath.Join("providers", provider, "models", file.Name())
				b, err := fs.ReadFile(path)
				if err != nil {
					loadErr = err
					return
				}
				var src modelFile
				if err := toml.Unmarshal(b, &src); err != nil {
					loadErr = fmt.Errorf("%s: %w", path, err)
					return
				}
				if src.Provider == "" {
					src.Provider = provider
				}
				if src.BaseURL == "" {
					src.BaseURL = providerDefaults.BaseURL
				}
				if len(src.Env) == 0 {
					src.Env = providerDefaults.Env
				}
				info := models.ModelInfo{
					Provider:      src.Provider,
					ID:            src.ID,
					API:           models.API(src.API),
					BaseURL:       src.BaseURL,
					Env:           src.Env,
					ContextWindow: src.ContextWindow,
					MaxOutput:     src.MaxOutput,
					Capabilities: models.ModelCapabilities{
						Tools:            src.Capabilities.Tools,
						Images:           src.Capabilities.Images,
						Reasoning:        src.Capabilities.Reasoning,
						StructuredOutput: src.Capabilities.StructuredOutput,
					},
				}
				ref := info.Provider + "/" + info.ID
				byRef[ref] = info
				if byProv[info.Provider] == nil {
					byProv[info.Provider] = map[string]models.ModelInfo{}
				}
				byProv[info.Provider][info.ID] = info
			}
		}
	})
	return loadErr
}

func readProviderFile(provider string) (providerFile, error) {
	path := filepath.Join("providers", provider, "provider.toml")
	b, err := fs.ReadFile(path)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return providerFile{}, nil
		}
		return providerFile{}, err
	}
	var src providerFile
	if err := toml.Unmarshal(b, &src); err != nil {
		return providerFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return src, nil
}

// GetRef returns metadata for a provider-qualified model ref.
func GetRef(ref string) (models.ModelInfo, bool) {
	overlayMu.RLock()
	if info, ok := overlayRef[ref]; ok {
		overlayMu.RUnlock()
		return info, true
	}
	overlayMu.RUnlock()

	if err := load(); err != nil {
		return models.ModelInfo{}, false
	}
	info, ok := byRef[ref]
	return info, ok
}

// GetModels returns the model catalog for a provider.
func GetModels(provider string) (map[string]models.ModelInfo, bool) {
	out := map[string]models.ModelInfo{}
	if err := load(); err != nil {
		return nil, false
	}
	if m, ok := byProv[provider]; ok {
		for id, info := range m {
			out[id] = info
		}
	}
	overlayMu.RLock()
	if m, ok := overlayProv[provider]; ok {
		for id, info := range m {
			out[id] = info
		}
	}
	overlayMu.RUnlock()
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// Get returns a single model's metadata.
func Get(provider, modelID string) (models.ModelInfo, bool) {
	return GetRef(provider + "/" + modelID)
}

// GetProviderBaseURL returns the catalog default base URL for a provider.
func GetProviderBaseURL(provider string) (string, bool) {
	overlayMu.RLock()
	if defaults, ok := overlayDefault[provider]; ok && defaults.BaseURL != "" {
		overlayMu.RUnlock()
		return defaults.BaseURL, true
	}
	overlayMu.RUnlock()

	if err := load(); err != nil {
		return "", false
	}
	defaults, ok := byDefault[provider]
	if !ok || defaults.BaseURL == "" {
		return "", false
	}
	return defaults.BaseURL, true
}

// RegisterProviderOverlay adds process-local provider defaults and model metadata.
// Config overlays win over the embedded catalog for the running daemon.
func RegisterProviderOverlay(provider string, baseURL string, modelsByID map[string]models.ModelInfo) {
	overlayMu.Lock()
	defer overlayMu.Unlock()
	if baseURL != "" {
		overlayDefault[provider] = providerFile{BaseURL: baseURL}
	}
	if len(modelsByID) == 0 {
		return
	}
	if overlayProv[provider] == nil {
		overlayProv[provider] = map[string]models.ModelInfo{}
	}
	for id, info := range modelsByID {
		if info.Provider == "" {
			info.Provider = provider
		}
		if info.ID == "" {
			info.ID = id
		}
		if info.BaseURL == "" {
			info.BaseURL = baseURL
		}
		if overlayProv[info.Provider] == nil {
			overlayProv[info.Provider] = map[string]models.ModelInfo{}
		}
		overlayProv[info.Provider][info.ID] = info
		overlayRef[info.Provider+"/"+info.ID] = info
	}
}
