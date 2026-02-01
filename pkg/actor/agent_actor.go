package actor

import (
	"context"
	"fmt"

	"wingman/pkg/agent"
	"wingman/pkg/provider"
)

const (
	MsgTypeWork   = "work"
	MsgTypeResult = "result"
)

type WorkPayload struct {
	Prompt string
	Data   any
}

type ResultPayload struct {
	Result *agent.Result
	Error  error
	Data   any
}

type AgentActor struct {
	agent    *agent.Agent
	provider provider.Provider
	target   *Ref
	onResult func(result *agent.Result, err error)
}

type AgentActorOption func(*AgentActor)

func WithTarget(target *Ref) AgentActorOption {
	return func(a *AgentActor) {
		a.target = target
	}
}

func WithResultCallback(fn func(result *agent.Result, err error)) AgentActorOption {
	return func(a *AgentActor) {
		a.onResult = fn
	}
}

func NewAgentActor(a *agent.Agent, p provider.Provider, opts ...AgentActorOption) *AgentActor {
	actor := &AgentActor{
		agent:    a,
		provider: p,
	}

	for _, opt := range opts {
		opt(actor)
	}

	return actor
}

func (a *AgentActor) Receive(ctx context.Context, msg Message) error {
	switch msg.Type {
	case MsgTypeWork:
		return a.handleWork(ctx, msg)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

func (a *AgentActor) handleWork(ctx context.Context, msg Message) error {
	payload, ok := msg.Payload.(WorkPayload)
	if !ok {
		return fmt.Errorf("invalid work payload")
	}

	result, err := a.agent.Run(ctx, a.provider, payload.Prompt)

	if a.onResult != nil {
		a.onResult(result, err)
	}

	if a.target != nil {
		resultMsg := NewMessage(msg.From, MsgTypeResult, ResultPayload{
			Result: result,
			Error:  err,
			Data:   payload.Data,
		})
		return a.target.Send(resultMsg)
	}

	return nil
}
