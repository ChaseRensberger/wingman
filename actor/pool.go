package actor

import (
	"context"
	"fmt"
	"sync"

	"wingman/agent"
	"wingman/provider"
	"wingman/session"
)

type Pool struct {
	system    *System
	workers   []*Ref
	collector *Ref
	results   []PoolResult
	resultsMu sync.Mutex
	done      chan struct{}
	expected  int
	received  int
}

type PoolResult struct {
	WorkerName string
	Result     *session.Result
	Error      error
	Data       any
}

type PoolConfig struct {
	WorkerCount int
	WorkDir     string
	Agent       *agent.Agent
	Provider    provider.Provider
}

func NewPool(cfg PoolConfig) *Pool {
	system := NewSystem()
	pool := &Pool{
		system:  system,
		workers: make([]*Ref, cfg.WorkerCount),
		results: []PoolResult{},
		done:    make(chan struct{}),
	}

	collector := &collectorActor{pool: pool}
	pool.collector = system.Spawn("collector", collector)

	for i := 0; i < cfg.WorkerCount; i++ {
		name := fmt.Sprintf("worker-%d", i)
		workerActor := NewAgentActor(
			cfg.Agent,
			cfg.Provider,
			WithTarget(pool.collector),
			WithWorkDir(cfg.WorkDir),
		)
		pool.workers[i] = system.Spawn(name, workerActor)
	}

	return pool
}

func (p *Pool) Submit(prompt string, data any) error {
	p.resultsMu.Lock()
	workerIdx := p.expected % len(p.workers)
	p.expected++
	p.resultsMu.Unlock()

	worker := p.workers[workerIdx]
	msg := NewMessage("pool", MsgTypeWork, WorkPayload{
		Prompt: prompt,
		Data:   data,
	})
	return worker.Send(msg)
}

func (p *Pool) SubmitAll(prompts []string) error {
	for i, prompt := range prompts {
		if err := p.Submit(prompt, i); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pool) AwaitAll() []PoolResult {
	<-p.done
	p.resultsMu.Lock()
	defer p.resultsMu.Unlock()
	return p.results
}

func (p *Pool) Shutdown() {
	p.system.Shutdown()
}

func (p *Pool) addResult(result PoolResult) {
	p.resultsMu.Lock()
	defer p.resultsMu.Unlock()

	p.results = append(p.results, result)
	p.received++

	if p.received >= p.expected {
		select {
		case <-p.done:
		default:
			close(p.done)
		}
	}
}

type collectorActor struct {
	pool *Pool
}

func (c *collectorActor) Receive(ctx context.Context, msg Message) error {
	if msg.Type != MsgTypeResult {
		return nil
	}

	payload, ok := msg.Payload.(ResultPayload)
	if !ok {
		return fmt.Errorf("invalid result payload")
	}

	c.pool.addResult(PoolResult{
		WorkerName: msg.From,
		Result:     payload.Result,
		Error:      payload.Error,
		Data:       payload.Data,
	})

	return nil
}
