package agent

import (
	"context"
	"fmt"

	"wingman/models"
	"wingman/provider"
	"wingman/session"
)

type Agent struct {
	name            string
	provider        provider.InferenceProvider
	providerFactory provider.ProviderFactory
	session         *session.Session
	config          models.WingmanConfig
	inbox           *Inbox
	id              string
}

type AgentOption func(*Agent) error

func CreateAgent(name string, opts ...AgentOption) (*Agent, error) {
	a := &Agent{
		name: name,
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	if a.providerFactory == nil {
		return nil, fmt.Errorf("provider is required")
	}

	p, err := a.providerFactory(a.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}
	a.provider = p

	a.session = session.CreateSession(a.provider)

	return a, nil
}

func WithProvider(factory provider.ProviderFactory) AgentOption {
	return func(a *Agent) error {
		a.providerFactory = factory
		return nil
	}
}

func WithInstructions(instructions string) AgentOption {
	return func(a *Agent) error {
		a.config.Instructions = instructions
		return nil
	}
}

func WithMaxTokens(maxTokens int) AgentOption {
	return func(a *Agent) error {
		a.config.MaxTokens = maxTokens
		return nil
	}
}

func WithTemperature(temperature float64) AgentOption {
	return func(a *Agent) error {
		a.config.Temperature = &temperature
		return nil
	}
}

func (a *Agent) RunInference(ctx context.Context, messages []models.WingmanMessage) (*models.WingmanMessageResponse, error) {
	if a.session == nil {
		return nil, fmt.Errorf("agent not properly initialized - no session")
	}

	result, err := a.provider.RunInference(ctx, messages, a.config)
	if err != nil {
		return nil, err
	}
	a.session.AddToSession(messages, result)
	return result, nil
}
