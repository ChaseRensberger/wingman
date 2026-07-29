// Package catalog loads the intentionally-small built-in model catalog.
package catalog

import (
	"embed"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/chaserensberger/wingman/models"
)

//go:embed labs/*/lab.toml labs/*/logo.svg models/*/*.toml providers/*/provider.toml providers/*/models/*.toml
var fs embed.FS

type modelFile struct {
	ID            string   `toml:"id"`
	Provider      string   `toml:"provider"`
	API           string   `toml:"api"`
	BaseURL       string   `toml:"base_url"`
	Env           []string `toml:"env"`
	ContextWindow int      `toml:"context_window"`
	MaxOutput     int      `toml:"max_output"`
	InputCost     float64  `toml:"input_cost_per_mtok"`
	OutputCost    float64  `toml:"output_cost_per_mtok"`
	BaseModel     string   `toml:"base_model"`
	Capabilities  struct {
		Tools            bool `toml:"tools"`
		Images           bool `toml:"images"`
		Reasoning        bool `toml:"reasoning"`
		StructuredOutput bool `toml:"structured_output"`
	} `toml:"capabilities"`
}

type providerFile struct {
	Name    string   `toml:"name"`
	Doc     string   `toml:"doc"`
	BaseURL string   `toml:"base_url"`
	Env     []string `toml:"env"`
}

type labFile struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Website     string `toml:"website"`
}

type canonicalModelFile struct {
	Lab         string `toml:"lab"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	ReleaseDate string `toml:"release_date"`
	LastUpdated string `toml:"last_updated"`
}

// LabInfo describes the organization that develops one or more catalog models.
type LabInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Website     string `json:"website,omitempty"`
	Logo        string `json:"logo,omitempty"`
}

// ModelMetadata describes one underlying model independently of its serving route.
type ModelMetadata struct {
	ID          string `json:"id"`
	Lab         string `json:"lab"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// ProviderInfo describes a catalog provider and its default route configuration.
type ProviderInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Doc     string   `json:"doc,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// RouteInfo describes a concrete provider/model route.
type RouteInfo struct {
	models.ModelInfo
	BaseModel string `json:"base_model,omitempty"`
}

var (
	loadOnce        sync.Once
	loadErr         error
	byRef           map[string]models.ModelInfo
	byProv          map[string]map[string]models.ModelInfo
	byDefault       map[string]providerFile
	labs            map[string]LabInfo
	canonicalModels map[string]ModelMetadata
	routes          []RouteInfo
	overlayMu       sync.RWMutex
	overlayRef      = map[string]models.ModelInfo{}
	overlayProv     = map[string]map[string]models.ModelInfo{}
	overlayDefault  = map[string]providerFile{}
)

func load() error {
	loadOnce.Do(func() {
		byRef = map[string]models.ModelInfo{}
		byProv = map[string]map[string]models.ModelInfo{}
		byDefault = map[string]providerFile{}
		labs = map[string]LabInfo{}
		canonicalModels = map[string]ModelMetadata{}
		if err := loadLabs(); err != nil {
			loadErr = err
			return
		}
		if err := loadCanonicalModels(); err != nil {
			loadErr = err
			return
		}
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
				if src.ID == "" || src.API == "" {
					loadErr = fmt.Errorf("%s: id and api are required", path)
					return
				}
				if src.BaseModel != "" {
					if _, ok := canonicalModels[src.BaseModel]; !ok {
						loadErr = fmt.Errorf("%s: unknown base_model %q", path, src.BaseModel)
						return
					}
				}
				info := models.ModelInfo{
					Provider:          src.Provider,
					ID:                src.ID,
					API:               models.API(src.API),
					BaseURL:           src.BaseURL,
					Env:               src.Env,
					ContextWindow:     src.ContextWindow,
					MaxOutput:         src.MaxOutput,
					InputCostPerMTok:  src.InputCost,
					OutputCostPerMTok: src.OutputCost,
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
				routes = append(routes, RouteInfo{ModelInfo: info, BaseModel: src.BaseModel})
			}
		}
	})
	return loadErr
}

func loadLabs() error {
	entries, err := fs.ReadDir("labs")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join("labs", entry.Name(), "lab.toml")
		b, err := fs.ReadFile(path)
		if err != nil {
			return err
		}
		var src labFile
		if err := toml.Unmarshal(b, &src); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if src.Name == "" || src.Description == "" {
			return fmt.Errorf("%s: name and description are required", path)
		}
		logoPath := filepath.Join("labs", entry.Name(), "logo.svg")
		logo := ""
		if _, err := fs.ReadFile(logoPath); err == nil {
			logo = "/catalog/labs/" + entry.Name() + "/logo"
		} else if !errors.Is(err, iofs.ErrNotExist) {
			return err
		}
		labs[entry.Name()] = LabInfo{ID: entry.Name(), Name: src.Name, Description: src.Description, Website: src.Website, Logo: logo}
	}
	return nil
}

func loadCanonicalModels() error {
	providers, err := fs.ReadDir("models")
	if err != nil {
		return err
	}
	for _, provider := range providers {
		if !provider.IsDir() {
			continue
		}
		files, err := fs.ReadDir(filepath.Join("models", provider.Name()))
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".toml") {
				continue
			}
			path := filepath.Join("models", provider.Name(), file.Name())
			b, err := fs.ReadFile(path)
			if err != nil {
				return err
			}
			var src canonicalModelFile
			if err := toml.Unmarshal(b, &src); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if src.Lab == "" || src.Name == "" || src.Description == "" {
				return fmt.Errorf("%s: lab, name, and description are required", path)
			}
			if _, ok := labs[src.Lab]; !ok {
				return fmt.Errorf("%s: unknown lab %q", path, src.Lab)
			}
			id := provider.Name() + "/" + strings.TrimSuffix(file.Name(), ".toml")
			canonicalModels[id] = ModelMetadata{ID: id, Lab: src.Lab, Name: src.Name, Description: src.Description, ReleaseDate: src.ReleaseDate, LastUpdated: src.LastUpdated}
		}
	}
	return nil
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

// ListLabs returns the labs represented in the embedded catalog.
func ListLabs() []LabInfo {
	if err := load(); err != nil {
		return nil
	}
	out := make([]LabInfo, 0, len(labs))
	for _, lab := range labs {
		out = append(out, lab)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListCanonicalModels returns model metadata independently of provider routes.
func ListCanonicalModels() []ModelMetadata {
	if err := load(); err != nil {
		return nil
	}
	out := make([]ModelMetadata, 0, len(canonicalModels))
	for _, model := range canonicalModels {
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListProviders returns the embedded catalog's provider metadata.
func ListProviders() []ProviderInfo {
	if err := load(); err != nil {
		return nil
	}
	out := make([]ProviderInfo, 0, len(byDefault))
	for id, provider := range byDefault {
		name := provider.Name
		if name == "" {
			name = id
		}
		out = append(out, ProviderInfo{ID: id, Name: name, Doc: provider.Doc, BaseURL: provider.BaseURL, Env: provider.Env})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListRoutes returns every embedded provider/model route.
func ListRoutes() []RouteInfo {
	if err := load(); err != nil {
		return nil
	}
	out := append([]RouteInfo(nil), routes...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Provider+"/"+out[i].ID < out[j].Provider+"/"+out[j].ID
	})
	return out
}

// LabLogo returns the embedded SVG logo for a lab.
func LabLogo(id string) ([]byte, bool) {
	if err := load(); err != nil {
		return nil, false
	}
	if _, ok := labs[id]; !ok {
		return nil, false
	}
	b, err := fs.ReadFile(filepath.Join("labs", id, "logo.svg"))
	if err != nil {
		return nil, false
	}
	return b, true
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
