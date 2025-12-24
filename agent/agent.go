package agent

import (
	"context"
	"fmt"
	"reflect"

	"wingman/models"
	"wingman/provider"
	"wingman/session"
)

type Agent struct {
	name         string
	provider     provider.InferenceProvider
	session      *session.Session
	instructions string
	inbox        *Inbox
	id           string
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

	if a.provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	a.session = session.CreateSession(a.provider)

	return a, nil
}

func WithProvider(factory any) AgentOption {
	return func(a *Agent) error {
		result := reflect.ValueOf(factory).Call(nil)
		if len(result) != 2 {
			return fmt.Errorf("provider factory must return (provider, error)")
		}

		if !result[1].IsNil() {
			return result[1].Interface().(error)
		}

		providerVal := result[0].Interface()
		inferenceProvider, ok := providerVal.(provider.InferenceProvider)
		if !ok {
			return fmt.Errorf("provider does not implement InferenceProvider interface")
		}

		a.provider = inferenceProvider
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
