package agent

import (
	"context"
	"fmt"

	"wingman/models"
	"wingman/provider"
	"wingman/session"
)

type Agent struct {
	name         string
	providerName string
	provider     provider.InferenceProvider
	session      *session.Session
	config       map[string]any
	instructions string
}

type AgentOption func(*Agent) error

func CreateAgent(name string, opts ...AgentOption) (*Agent, error) {
	a := &Agent{
		name:   name,
		config: make(map[string]any),
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	if err := a.initialize(); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *Agent) initialize() error {
	if a.providerName == "" {
		return fmt.Errorf("provider is required")
	}

	inferenceProvider, err := provider.GetProviderFromRegistry(a.providerName, a.config)
	if err != nil {
		return fmt.Errorf("failed to create provider %s: %w", a.providerName, err)
	}

	a.provider = inferenceProvider
	a.session = session.CreateSession(a.provider)
	return nil
}

func WithProvider(providerName string) AgentOption {
	return func(a *Agent) error {
		a.providerName = providerName
		return nil
	}
}

func WithConfig(cfg map[string]any) AgentOption {
	return func(a *Agent) error {
		a.config = cfg
		return nil
	}
}

func WithInstructions(instructions string) AgentOption {
	return func(a *Agent) error {
		a.instructions = instructions
		return nil
	}
}

func (a *Agent) RunInference(ctx context.Context, messages []models.WingmanMessage) (*models.WingmanMessageResponse, error) {
	if a.session == nil {
		return nil, fmt.Errorf("agent not properly initialized - no session")
	}

	result, err := a.provider.RunInference(ctx, messages, a.instructions)
	if err != nil {
		return nil, err
	}
	a.session.AddToSession(messages, result)
	return result, nil
}
