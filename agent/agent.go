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
	provider     provider.InferenceProvider
	session      *session.Session
	config       map[string]any
	providerName string
	instructions string
}

func CreateAgent(name string) *Agent {
	return &Agent{
		name:   name,
		config: make(map[string]any),
	}
}

func (a *Agent) WithProvider(providerName string) *Agent {
	a.providerName = providerName
	inferenceProvider, err := provider.GetProviderFromRegistry(providerName, a.config)
	if err != nil {
		panic(err)
	}
	a.provider = inferenceProvider
	a.session = session.CreateSession(inferenceProvider)
	return a
}

func (a *Agent) WithConfig(cfg map[string]any) *Agent {
	a.config = cfg
	// Recreate session with new config if provider is already set
	if a.provider != nil && a.providerName != "" {
		inferenceProvider, err := provider.GetProviderFromRegistry(a.providerName, a.config)
		if err != nil {
			panic(err)
		}
		a.provider = inferenceProvider
		a.session = session.CreateSession(inferenceProvider)
	}
	return a
}

func (a *Agent) WithInstructions(instructions string) *Agent {
	a.instructions = instructions
	return a
}

func (a *Agent) WithStructuredOutput(schema map[string]any) *Agent {
	a.config["output_schema"] = schema
	if a.provider != nil && a.providerName != "" {
		inferenceProvider, err := provider.GetProviderFromRegistry(a.providerName, a.config)
		if err != nil {
			panic(err)
		}
		a.provider = inferenceProvider
		a.session = session.CreateSession(inferenceProvider)
	}
	return a
}

func (a *Agent) RunInference(ctx context.Context, messages []models.WingmanMessage) (*models.WingmanMessageResponse, error) {
	if a.session == nil {
		return nil, fmt.Errorf("agent not properly initialized - no session")
	}
	return a.session.RunInference(ctx, messages, a.instructions)
}
