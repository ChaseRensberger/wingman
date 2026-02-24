package agent

import (
	"github.com/chaserensberger/wingman/core"
)

type Agent struct {
	id           string
	name         string
	instructions string
	tools        []core.Tool
	outputSchema map[string]any
	provider     core.Provider
	providerID   string
	model        string
}

type Option func(*Agent)

func New(name string, opts ...Option) *Agent {
	a := &Agent{name: name}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithID(id string) Option {
	return func(a *Agent) { a.id = id }
}

func WithInstructions(instructions string) Option {
	return func(a *Agent) { a.instructions = instructions }
}

func WithTools(tools ...core.Tool) Option {
	return func(a *Agent) { a.tools = append(a.tools, tools...) }
}

func WithOutputSchema(schema map[string]any) Option {
	return func(a *Agent) { a.outputSchema = schema }
}

func WithProvider(p core.Provider) Option {
	return func(a *Agent) { a.provider = p }
}

func WithProviderID(id string) Option {
	return func(a *Agent) { a.providerID = id }
}

func WithModel(model string) Option {
	return func(a *Agent) { a.model = model }
}

func (a *Agent) ID() string                   { return a.id }
func (a *Agent) Name() string                 { return a.name }
func (a *Agent) Instructions() string         { return a.instructions }
func (a *Agent) Tools() []core.Tool           { return a.tools }
func (a *Agent) OutputSchema() map[string]any { return a.outputSchema }
func (a *Agent) Provider() core.Provider      { return a.provider }
func (a *Agent) ProviderID() string           { return a.providerID }
func (a *Agent) Model() string                { return a.model }

func (a *Agent) SetProvider(p core.Provider) { a.provider = p }
