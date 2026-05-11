package catalog

import "time"

type Source struct {
	Label       string `json:"label"`
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
}

type Lab struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Sources     []Source `json:"sources,omitempty"`
}

type LabModel struct {
	ID              string      `json:"id"`
	LabID           string      `json:"lab_id"`
	DisplayName     string      `json:"display_name"`
	Family          string      `json:"family,omitempty"`
	ReleaseDate     string      `json:"release_date,omitempty"`
	KnowledgeCutoff string      `json:"knowledge_cutoff,omitempty"`
	Modalities      *Modalities `json:"modalities,omitempty"`
	OpenWeights     bool        `json:"open_weights"`
	Sources         []Source    `json:"sources,omitempty"`
}

type ProviderAuth struct {
	EnvVars []string `json:"env_vars"`
}

type Provider struct {
	ID          string        `json:"id"`
	DisplayName string        `json:"display_name"`
	BaseURLs    []string      `json:"base_urls,omitempty"`
	Auth        *ProviderAuth `json:"auth,omitempty"`
	Sources     []Source      `json:"sources,omitempty"`
}

type CapabilityInstance struct {
	ID      CapabilityID   `json:"id"`
	Version string         `json:"version,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type Limits struct {
	ContextWindow   int `json:"context_window,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	MaxInputTokens  int `json:"max_input_tokens,omitempty"`
}

type Pricing struct {
	InputPerMillion      float64 `json:"input_per_million,omitempty"`
	OutputPerMillion     float64 `json:"output_per_million,omitempty"`
	CacheReadPerMillion  float64 `json:"cache_read_per_million,omitempty"`
	CacheWritePerMillion float64 `json:"cache_write_per_million,omitempty"`
}

type ProviderModel struct {
	ID                  string               `json:"id"`
	DisplayName         string               `json:"display_name"`
	InterfaceProfiles   []string             `json:"interface_profiles,omitempty"`
	Capabilities        []CapabilityInstance `json:"capabilities,omitempty"`
	SupportedParameters []ParameterID        `json:"supported_parameters,omitempty"`
	Limits              *Limits              `json:"limits,omitempty"`
	Pricing             *Pricing             `json:"pricing,omitempty"`
	Modalities          *Modalities          `json:"modalities,omitempty"`
	Sources             []Source             `json:"sources,omitempty"`
	Extensions          map[string]any       `json:"extensions,omitempty"`
}

type Snapshot struct {
	Version        string          `json:"version"`
	GeneratedAt    time.Time       `json:"generated_at"`
	Labs           []Lab           `json:"labs"`
	LabModels      []LabModel      `json:"lab_models"`
	Providers      []Provider      `json:"providers"`
	ProviderModels []ProviderModel `json:"provider_models"`
}
