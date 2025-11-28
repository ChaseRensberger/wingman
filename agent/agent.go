package agent

import (
	"wingman/provider"
)

type AgentBuilder struct {
	name   string
	config map[string]any
}

type Agent struct {
	name     string
	provider provider.InferenceProvider
	config   map[string]any
}

func CreateAgent(name string) *Agent {
	return &Agent{
		name: name,
	}
}
