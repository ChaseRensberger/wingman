package agent

import (
	"wingman/provider"
	"wingman/tool"
)

type Agent struct {
	name         string
	instructions string
	tools        []tool.Tool
	outputSchema map[string]any
	provider     provider.Provider
}

type Option func(*Agent)

func New(name string, opts ...Option) *Agent {
	a := &Agent{
		name: name,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

func WithInstructions(instructions string) Option {
	return func(a *Agent) {
		a.instructions = instructions
	}
}

func WithTools(tools ...tool.Tool) Option {
	return func(a *Agent) {
		a.tools = append(a.tools, tools...)
	}
}

func WithOutputSchema(schema map[string]any) Option {
	return func(a *Agent) {
		a.outputSchema = schema
	}
}

func WithProvider(p provider.Provider) Option {
	return func(a *Agent) {
		a.provider = p
	}
}

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) Instructions() string {
	return a.instructions
}

func (a *Agent) Tools() []tool.Tool {
	return a.tools
}

func (a *Agent) OutputSchema() map[string]any {
	return a.outputSchema
}

func (a *Agent) Provider() provider.Provider {
	return a.provider
}
