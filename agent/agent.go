package agent

import (
	"wingman/tool"
)

type Agent struct {
	name         string
	instructions string
	tools        []tool.Tool
	maxTokens    int
	temperature  *float64
	maxSteps     int
	outputSchema map[string]any
}

type Option func(*Agent)

func New(name string, opts ...Option) *Agent {
	a := &Agent{
		name:     name,
		maxSteps: 50,
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

func WithMaxTokens(maxTokens int) Option {
	return func(a *Agent) {
		a.maxTokens = maxTokens
	}
}

func WithTemperature(temperature float64) Option {
	return func(a *Agent) {
		a.temperature = &temperature
	}
}

func WithMaxSteps(maxSteps int) Option {
	return func(a *Agent) {
		a.maxSteps = maxSteps
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

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) Instructions() string {
	return a.instructions
}

func (a *Agent) Tools() []tool.Tool {
	return a.tools
}

func (a *Agent) MaxTokens() int {
	return a.maxTokens
}

func (a *Agent) Temperature() *float64 {
	return a.temperature
}

func (a *Agent) MaxSteps() int {
	return a.maxSteps
}

func (a *Agent) OutputSchema() map[string]any {
	return a.outputSchema
}
