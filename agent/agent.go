// Package agent defines the Agent type — the central config object that binds
// a provider, tool set, and instructions together. Agents are passed to
// sessions (or fleets) to drive inference.
package agent

import (
	"github.com/chaserensberger/wingman/core"
)

// Agent holds the configuration for a single LLM-driven agent. All fields are
// unexported; use the functional option constructors to build agents and the
// getter methods to read them.
//
// In SDK usage, agents are constructed in Go code and passed directly to
// sessions. In server usage, agents are persisted in SQLite (with provider and
// model as strings) and reconstructed via the provider registry on each
// inference request.
type Agent struct {
	id           string
	name         string
	instructions string
	tools        []core.Tool
	outputSchema map[string]any
	provider     core.Provider

	// ProviderID and Model are set when the agent was created from a stored
	// definition (server path or registry-based SDK usage). They are
	// informational and not required when a live provider is attached directly.
	providerID string
	model      string
}

// Option is a functional option for New.
type Option func(*Agent)

// New creates an Agent with the given name and options.
func New(name string, opts ...Option) *Agent {
	a := &Agent{name: name}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ============================================================
//  Option constructors
// ============================================================

// WithID sets an explicit ID on the agent (used when reconstructing from storage).
func WithID(id string) Option {
	return func(a *Agent) { a.id = id }
}

// WithInstructions sets the system prompt sent to the model on every inference call.
func WithInstructions(instructions string) Option {
	return func(a *Agent) { a.instructions = instructions }
}

// WithTools appends tools to the agent's tool list. Can be called multiple times.
func WithTools(tools ...core.Tool) Option {
	return func(a *Agent) { a.tools = append(a.tools, tools...) }
}

// WithOutputSchema sets a JSON Schema for structured output.
func WithOutputSchema(schema map[string]any) Option {
	return func(a *Agent) { a.outputSchema = schema }
}

// WithProvider attaches a live Provider instance to the agent.
func WithProvider(p core.Provider) Option {
	return func(a *Agent) { a.provider = p }
}

// WithProviderID records the string provider identifier (e.g. "anthropic").
// This is informational when a live provider is already attached; required
// when using the agent with the server storage layer.
func WithProviderID(id string) Option {
	return func(a *Agent) { a.providerID = id }
}

// WithModel records the model identifier (e.g. "claude-opus-4-6").
// This is informational when a live provider is already attached; required
// when using the agent with the server storage layer.
func WithModel(model string) Option {
	return func(a *Agent) { a.model = model }
}

// ============================================================
//  Getters
// ============================================================

func (a *Agent) ID() string                   { return a.id }
func (a *Agent) Name() string                 { return a.name }
func (a *Agent) Instructions() string         { return a.instructions }
func (a *Agent) Tools() []core.Tool           { return a.tools }
func (a *Agent) OutputSchema() map[string]any { return a.outputSchema }
func (a *Agent) Provider() core.Provider      { return a.provider }
func (a *Agent) ProviderID() string           { return a.providerID }
func (a *Agent) Model() string                { return a.model }

// ============================================================
//  Setters (used by session to update agent at runtime)
// ============================================================

// SetProvider replaces the live provider on the agent.
func (a *Agent) SetProvider(p core.Provider) { a.provider = p }
