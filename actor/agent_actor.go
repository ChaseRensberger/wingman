package actor

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/session"
)

const (
	MsgTypeWork   = "work"
	MsgTypeResult = "result"
)

type WorkPayload struct {
	Message string
	Data    any
}

type ResultPayload struct {
	Result *session.Result
	Error  error
	Data   any
}

type AgentActor struct {
	agent    *agent.Agent
	workDir  string
	target   *Ref
	onResult func(result *session.Result, err error)
}

type AgentActorOption func(*AgentActor)

func WithTarget(target *Ref) AgentActorOption {
	return func(a *AgentActor) {
		a.target = target
	}
}

func WithWorkDir(dir string) AgentActorOption {
	return func(a *AgentActor) {
		a.workDir = dir
	}
}

func WithResultCallback(fn func(result *session.Result, err error)) AgentActorOption {
	return func(a *AgentActor) {
		a.onResult = fn
	}
}

func NewAgentActor(a *agent.Agent, opts ...AgentActorOption) *AgentActor {
	actor := &AgentActor{
		agent: a,
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

	s := session.New(
		session.WithAgent(a.agent),
		session.WithWorkDir(a.workDir),
	)

	result, err := s.Run(ctx, payload.Message)

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
