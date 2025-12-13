package agent

import (
	"context"
	"fmt"
	"log"
	"wingman/models"
	"wingman/provider"
	"wingman/session"
	"wingman/utils"
)

type Agent struct {
	name         string
	provider     provider.InferenceProvider
	session      *session.Session
	config       map[string]any
	instructions string
}

func CreateAgent(name string) *Agent {
	utils.Logger.Debug("Creating agent...", "name", name)
	return &Agent{
		name:   name,
		config: make(map[string]any),
	}
}

func (a *Agent) WithProvider(providerName string) *Agent {
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
	if a.provider != nil {
		inferenceProvider, err := provider.GetProviderFromRegistry(a.provider.Name(), a.config)
		if err != nil {
			log.Fatal(err)
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

func (a *Agent) RunInference(ctx context.Context, messages []models.WingmanMessage) (*models.WingmanMessageResponse, error) {
	if a.session == nil {
		return nil, fmt.Errorf("agent not properly initialized - no session")
	}

	result, err := a.provider.RunInference(ctx, messages, a.instructions)
	if err != nil {
		return nil, err
	}
	a.session.AddToHistory(messages, result)
	return result, nil
}
