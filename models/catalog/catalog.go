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
	ID            string                `toml:"id"`
	Provider      string                `toml:"provider"`
	API           string                `toml:"api"`
	BaseURL       string                `toml:"base_url"`
	Env           []string              `toml:"env"`
	ContextWindow int                   `toml:"context_window"`
	MaxOutput     int                   `toml:"max_output"`
	InputCost     float64               `toml:"input_cost_per_mtok"`
	OutputCost    float64               `toml:"output_cost_per_mtok"`
	BaseModel     string                `toml:"base_model"`
	Variants      []models.ModelVariant `toml:"variants"`
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

// ProviderOverlay supplies per-generation provider defaults and model routes.
type ProviderOverlay struct {
	BaseURL string
	Models  map[string]models.ModelInfo
}

// Catalog is an immutable snapshot of model routes and provider defaults.
type Catalog struct {
	byRef           map[string]models.ModelInfo
	byProv          map[string]map[string]models.ModelInfo
	byDefault       map[string]providerFile
	labs            map[string]LabInfo
	canonicalModels map[string]ModelMetadata
	routes          []RouteInfo
}

var (
	loadOnce sync.Once
	loadErr  error
	builtin  *Catalog
)

func load() error {
	loadOnce.Do(func() {
		builtin = &Catalog{byRef: map[string]models.ModelInfo{}, byProv: map[string]map[string]models.ModelInfo{}, byDefault: map[string]providerFile{}, labs: map[string]LabInfo{}, canonicalModels: map[string]ModelMetadata{}}
		loadErr = loadEmbedded(builtin)
	})
	return loadErr
}

func loadEmbedded(c *Catalog) error {
	if err := loadLabs(c); err != nil {
		return err
	}
	if err := loadCanonicalModels(c); err != nil {
		return err
	}
	entries, err := fs.ReadDir("providers")
	if err != nil {
		return err
	}
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		provider := dir.Name()
		defaults, err := readProviderFile(provider)
		if err != nil {
			return err
		}
		c.byDefault[provider] = defaults
		files, err := fs.ReadDir(filepath.Join("providers", provider, "models"))
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".toml") {
				continue
			}
			path := filepath.Join("providers", provider, "models", file.Name())
			b, err := fs.ReadFile(path)
			if err != nil {
				return err
			}
			var src modelFile
			if err := toml.Unmarshal(b, &src); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if src.Provider == "" {
				src.Provider = provider
			}
			if src.BaseURL == "" {
				src.BaseURL = defaults.BaseURL
			}
			if len(src.Env) == 0 {
				src.Env = defaults.Env
			}
			if src.ID == "" || src.API == "" {
				return fmt.Errorf("%s: id and api are required", path)
			}
			if src.BaseModel != "" {
				if _, ok := c.canonicalModels[src.BaseModel]; !ok {
					return fmt.Errorf("%s: unknown base_model %q", path, src.BaseModel)
				}
			}
			if err := validateVariants(src.Variants); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			info := models.ModelInfo{Provider: src.Provider, ID: src.ID, API: models.API(src.API), BaseURL: src.BaseURL, Env: append([]string(nil), src.Env...), ContextWindow: src.ContextWindow, MaxOutput: src.MaxOutput, InputCostPerMTok: src.InputCost, OutputCostPerMTok: src.OutputCost, Variants: cloneVariants(src.Variants), Capabilities: models.ModelCapabilities{Tools: src.Capabilities.Tools, Images: src.Capabilities.Images, Reasoning: src.Capabilities.Reasoning, StructuredOutput: src.Capabilities.StructuredOutput}}
			c.addRoute(info, src.BaseModel)
		}
	}
	return nil
}

func (c *Catalog) addRoute(info models.ModelInfo, baseModel string) {
	info.Env = append([]string(nil), info.Env...)
	info.Variants = cloneVariants(info.Variants)
	c.byRef[info.Provider+"/"+info.ID] = info
	if c.byProv[info.Provider] == nil {
		c.byProv[info.Provider] = map[string]models.ModelInfo{}
	}
	c.byProv[info.Provider][info.ID] = info
	for i := range c.routes {
		if c.routes[i].Provider == info.Provider && c.routes[i].ID == info.ID {
			if baseModel == "" {
				baseModel = c.routes[i].BaseModel
			}
			c.routes[i] = RouteInfo{ModelInfo: info, BaseModel: baseModel}
			return
		}
	}
	c.routes = append(c.routes, RouteInfo{ModelInfo: info, BaseModel: baseModel})
}
func loadLabs(c *Catalog) error {
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
		c.labs[entry.Name()] = LabInfo{ID: entry.Name(), Name: src.Name, Description: src.Description, Website: src.Website, Logo: logo}
	}
	return nil
}
func loadCanonicalModels(c *Catalog) error {
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
			if _, ok := c.labs[src.Lab]; !ok {
				return fmt.Errorf("%s: unknown lab %q", path, src.Lab)
			}
			id := provider.Name() + "/" + strings.TrimSuffix(file.Name(), ".toml")
			c.canonicalModels[id] = ModelMetadata{ID: id, Lab: src.Lab, Name: src.Name, Description: src.Description, ReleaseDate: src.ReleaseDate, LastUpdated: src.LastUpdated}
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

// Builtin returns the immutable embedded catalog.
func Builtin() *Catalog {
	if load() != nil {
		return nil
	}
	return builtin
}

// New builds an immutable catalog snapshot by applying overlays to the embedded catalog.
func New(overlays map[string]ProviderOverlay) (*Catalog, error) {
	if err := load(); err != nil {
		return nil, err
	}
	c := builtin.clone()
	ids := make([]string, 0, len(overlays))
	for id := range overlays {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("provider ID is required")
		}
		overlay := overlays[id]
		defaults := c.byDefault[id]
		if overlay.BaseURL != "" {
			defaults.BaseURL = overlay.BaseURL
			for _, info := range c.byProv[id] {
				info.BaseURL = overlay.BaseURL
				c.addRoute(info, "")
			}
		}
		c.byDefault[id] = defaults
		modelIDs := make([]string, 0, len(overlay.Models))
		for modelID := range overlay.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)
		for _, modelID := range modelIDs {
			info := overlay.Models[modelID]
			if modelID == "" || info.ID == "" {
				return nil, fmt.Errorf("provider %q: model ID is required", id)
			}
			if info.Provider != id || info.ID != modelID {
				return nil, fmt.Errorf("provider %q: model %q identity does not match its map key", id, modelID)
			}
			if info.BaseURL == "" {
				info.BaseURL = defaults.BaseURL
			}
			if info.BaseURL == "" {
				return nil, fmt.Errorf("provider %q model %q: base URL is required", id, modelID)
			}
			if err := validateVariants(info.Variants); err != nil {
				return nil, fmt.Errorf("provider %q model %q: %w", id, modelID, err)
			}
			info.Env = append([]string(nil), info.Env...)
			c.addRoute(info, "")
		}
	}
	return c, nil
}

func validateVariants(variants []models.ModelVariant) error {
	seen := make(map[string]struct{}, len(variants))
	for _, variant := range variants {
		if variant.ID == "" {
			return errors.New("variant ID is required")
		}
		if _, exists := seen[variant.ID]; exists {
			return fmt.Errorf("duplicate variant %q", variant.ID)
		}
		seen[variant.ID] = struct{}{}
	}
	return nil
}
func (c *Catalog) clone() *Catalog {
	out := &Catalog{byRef: map[string]models.ModelInfo{}, byProv: map[string]map[string]models.ModelInfo{}, byDefault: map[string]providerFile{}, labs: map[string]LabInfo{}, canonicalModels: map[string]ModelMetadata{}, routes: make([]RouteInfo, len(c.routes))}
	for i, route := range c.routes {
		route.Env = append([]string(nil), route.Env...)
		route.Variants = cloneVariants(route.Variants)
		out.routes[i] = route
	}
	for k, v := range c.byRef {
		v.Env = append([]string(nil), v.Env...)
		v.Variants = cloneVariants(v.Variants)
		out.byRef[k] = v
	}
	for p, m := range c.byProv {
		out.byProv[p] = map[string]models.ModelInfo{}
		for id, v := range m {
			v.Env = append([]string(nil), v.Env...)
			v.Variants = cloneVariants(v.Variants)
			out.byProv[p][id] = v
		}
	}
	for k, v := range c.byDefault {
		v.Env = append([]string(nil), v.Env...)
		out.byDefault[k] = v
	}
	for k, v := range c.labs {
		out.labs[k] = v
	}
	for k, v := range c.canonicalModels {
		out.canonicalModels[k] = v
	}
	return out
}

// GetRef returns metadata for a provider-qualified model ref.
func (c *Catalog) GetRef(ref string) (models.ModelInfo, bool) {
	info, ok := c.byRef[ref]
	if ok {
		info.Env = append([]string(nil), info.Env...)
		info.Variants = cloneVariants(info.Variants)
	}
	return info, ok
}

// GetModels returns the model catalog for a provider.
func (c *Catalog) GetModels(provider string) (map[string]models.ModelInfo, bool) {
	m, ok := c.byProv[provider]
	if !ok {
		return nil, false
	}
	out := make(map[string]models.ModelInfo, len(m))
	for id, info := range m {
		info.Env = append([]string(nil), info.Env...)
		info.Variants = cloneVariants(info.Variants)
		out[id] = info
	}
	return out, true
}

// Get returns a single model's metadata.
func (c *Catalog) Get(provider, modelID string) (models.ModelInfo, bool) {
	return c.GetRef(provider + "/" + modelID)
}

// GetProviderBaseURL returns the catalog default base URL for a provider.
func (c *Catalog) GetProviderBaseURL(provider string) (string, bool) {
	defaults, ok := c.byDefault[provider]
	return defaults.BaseURL, ok && defaults.BaseURL != ""
}

// ListLabs returns the labs represented in the catalog.
func (c *Catalog) ListLabs() []LabInfo {
	out := make([]LabInfo, 0, len(c.labs))
	for _, lab := range c.labs {
		out = append(out, lab)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListCanonicalModels returns model metadata independently of provider routes.
func (c *Catalog) ListCanonicalModels() []ModelMetadata {
	out := make([]ModelMetadata, 0, len(c.canonicalModels))
	for _, model := range c.canonicalModels {
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListProviders returns the catalog's provider metadata.
func (c *Catalog) ListProviders() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(c.byDefault))
	for id, provider := range c.byDefault {
		name := provider.Name
		if name == "" {
			name = id
		}
		out = append(out, ProviderInfo{ID: id, Name: name, Doc: provider.Doc, BaseURL: provider.BaseURL, Env: append([]string(nil), provider.Env...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListRoutes returns every provider/model route.
func (c *Catalog) ListRoutes() []RouteInfo {
	out := append([]RouteInfo(nil), c.routes...)
	for i := range out {
		out[i].Env = append([]string(nil), out[i].Env...)
		out[i].Variants = cloneVariants(out[i].Variants)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider+"/"+out[i].ID < out[j].Provider+"/"+out[j].ID })
	return out
}

func cloneVariants(in []models.ModelVariant) []models.ModelVariant {
	if in == nil {
		return nil
	}
	out := make([]models.ModelVariant, len(in))
	for i, variant := range in {
		out[i] = variant
		out[i].ProviderOptions = cloneJSONMap(variant.ProviderOptions)
		out[i].HTTP.Headers = cloneStringMap(variant.HTTP.Headers)
		out[i].HTTP.Query = cloneStringMap(variant.HTTP.Query)
		out[i].HTTP.Body = cloneJSONMap(variant.HTTP.Body)
	}
	return out
}

func cloneJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneJSONMap(value)
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = cloneJSONValue(item)
		}
		return out
	default:
		return value
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// LabLogo returns the embedded SVG logo for a lab.
func (c *Catalog) LabLogo(id string) ([]byte, bool) {
	if _, ok := c.labs[id]; !ok {
		return nil, false
	}
	b, err := fs.ReadFile(filepath.Join("labs", id, "logo.svg"))
	return b, err == nil
}

// Package-level functions provide compatibility access to the built-in catalog.
func GetRef(ref string) (models.ModelInfo, bool) {
	c := Builtin()
	if c == nil {
		return models.ModelInfo{}, false
	}
	return c.GetRef(ref)
}
func GetModels(provider string) (map[string]models.ModelInfo, bool) {
	c := Builtin()
	if c == nil {
		return nil, false
	}
	return c.GetModels(provider)
}
func Get(provider, modelID string) (models.ModelInfo, bool) { return GetRef(provider + "/" + modelID) }
func GetProviderBaseURL(provider string) (string, bool) {
	c := Builtin()
	if c == nil {
		return "", false
	}
	return c.GetProviderBaseURL(provider)
}
func ListLabs() []LabInfo {
	c := Builtin()
	if c == nil {
		return nil
	}
	return c.ListLabs()
}
func ListCanonicalModels() []ModelMetadata {
	c := Builtin()
	if c == nil {
		return nil
	}
	return c.ListCanonicalModels()
}
func ListProviders() []ProviderInfo {
	c := Builtin()
	if c == nil {
		return nil
	}
	return c.ListProviders()
}
func ListRoutes() []RouteInfo {
	c := Builtin()
	if c == nil {
		return nil
	}
	return c.ListRoutes()
}
func LabLogo(id string) ([]byte, bool) {
	c := Builtin()
	if c == nil {
		return nil, false
	}
	return c.LabLogo(id)
}
