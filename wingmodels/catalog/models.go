package models_dev

type ModelsDB map[string]ProviderData

type ProviderData struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Env    []string         `json:"env"`
	NPM    string           `json:"npm"`
	API    string           `json:"api,omitempty"`
	Doc    string           `json:"doc"`
	Models map[string]Model `json:"models"`
}

type Model struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Family           string     `json:"family,omitempty"`
	Attachment       bool       `json:"attachment"`
	Reasoning        bool       `json:"reasoning"`
	ToolCall         bool       `json:"tool_call"`
	StructuredOutput bool       `json:"structured_output,omitempty"`
	Temperature      bool       `json:"temperature,omitempty"`
	Knowledge        string     `json:"knowledge,omitempty"`
	ReleaseDate      string     `json:"release_date,omitempty"`
	LastUpdated      string     `json:"last_updated,omitempty"`
	OpenWeights      bool       `json:"open_weights,omitempty"`
	Status           string     `json:"status,omitempty"`
	Interleaved      any        `json:"interleaved,omitempty"`
	Cost             ModelCost  `json:"cost"`
	Limit            ModelLimit `json:"limit"`
	Modalities       Modalities `json:"modalities"`
}

type ModelCost struct {
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	Reasoning   float64 `json:"reasoning,omitempty"`
	CacheRead   float64 `json:"cache_read,omitempty"`
	CacheWrite  float64 `json:"cache_write,omitempty"`
	InputAudio  float64 `json:"input_audio,omitempty"`
	OutputAudio float64 `json:"output_audio,omitempty"`
}

type ModelLimit struct {
	Context int `json:"context"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}
