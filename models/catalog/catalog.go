// Package catalog loads the intentionally-small built-in model catalog.
package catalog

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/chaserensberger/wingman/models"
)

//go:embed providers/**/*.toml
var fs embed.FS

type modelFile struct {
	ID            string `toml:"id"`
	Provider      string `toml:"provider"`
	API           string `toml:"api"`
	BaseURL       string `toml:"base_url"`
	AuthEnv       string `toml:"auth_env"`
	ContextWindow int    `toml:"context_window"`
	MaxOutput     int    `toml:"max_output"`
	Capabilities  struct {
		Tools            bool `toml:"tools"`
		Images           bool `toml:"images"`
		Reasoning        bool `toml:"reasoning"`
		StructuredOutput bool `toml:"structured_output"`
	} `toml:"capabilities"`
}

var (
	loadOnce sync.Once
	loadErr  error
	byRef    map[string]models.ModelInfo
	byProv   map[string]map[string]models.ModelInfo
)

func load() error {
	loadOnce.Do(func() {
		byRef = map[string]models.ModelInfo{}
		byProv = map[string]map[string]models.ModelInfo{}
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
			files, err := fs.ReadDir(filepath.Join("providers", provider))
			if err != nil {
				loadErr = err
				return
			}
			for _, file := range files {
				if file.IsDir() || !strings.HasSuffix(file.Name(), ".toml") {
					continue
				}
				path := filepath.Join("providers", provider, file.Name())
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
				info := models.ModelInfo{
					Provider:      src.Provider,
					ID:            src.ID,
					API:           models.API(src.API),
					BaseURL:       src.BaseURL,
					AuthEnv:       src.AuthEnv,
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

// GetRef returns metadata for a provider-qualified model ref.
func GetRef(ref string) (models.ModelInfo, bool) {
	if err := load(); err != nil {
		return models.ModelInfo{}, false
	}
	info, ok := byRef[ref]
	return info, ok
}

// GetModels returns the model catalog for a provider.
func GetModels(provider string) (map[string]models.ModelInfo, bool) {
	if err := load(); err != nil {
		return nil, false
	}
	m, ok := byProv[provider]
	return m, ok
}

// Get returns a single model's metadata.
func Get(provider, modelID string) (models.ModelInfo, bool) {
	return GetRef(provider + "/" + modelID)
}
