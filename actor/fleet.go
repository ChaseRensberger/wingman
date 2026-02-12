package actor

import (
	"context"
	"fmt"
	"sync"

	"wingman/agent"
	"wingman/session"
)

type Fleet struct {
	system    *System
	workers   []*Ref
	collector *Ref
	results   []FleetResult
	resultsMu sync.Mutex
	done      chan struct{}
	expected  int
	received  int
}

type FleetResult struct {
	WorkerName string
	Result     *session.Result
	Error      error
	Data       any
}

type FleetConfig struct {
	WorkerCount int
	WorkDir     string
	Agent       *agent.Agent
}

func NewFleet(cfg FleetConfig) *Fleet {
	system := NewSystem()
	fleet := &Fleet{
		system:  system,
		workers: make([]*Ref, cfg.WorkerCount),
		results: []FleetResult{},
		done:    make(chan struct{}),
	}

	collector := &collectorActor{fleet: fleet}
	fleet.collector = system.Spawn("collector", collector)

	for i := 0; i < cfg.WorkerCount; i++ {
		name := fmt.Sprintf("worker-%d", i)
		workerActor := NewAgentActor(
			cfg.Agent,
			WithTarget(fleet.collector),
			WithWorkDir(cfg.WorkDir),
		)
		fleet.workers[i] = system.Spawn(name, workerActor)
	}

	return fleet
}

func (f *Fleet) Submit(message string, data any) error {
	f.resultsMu.Lock()
	workerIdx := f.expected % len(f.workers)
	f.expected++
	f.resultsMu.Unlock()

	worker := f.workers[workerIdx]
	msg := NewMessage("fleet", MsgTypeWork, WorkPayload{
		Message: message,
		Data:    data,
	})
	return worker.Send(msg)
}

func (f *Fleet) SubmitAll(messages []string) error {
	for i, message := range messages {
		if err := f.Submit(message, i); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fleet) AwaitAll() []FleetResult {
	<-f.done
	f.resultsMu.Lock()
	defer f.resultsMu.Unlock()
	return f.results
}

func (f *Fleet) Shutdown() {
	f.system.Shutdown()
}

func (f *Fleet) addResult(result FleetResult) {
	f.resultsMu.Lock()
	defer f.resultsMu.Unlock()

	f.results = append(f.results, result)
	f.received++

	if f.received >= f.expected {
		select {
		case <-f.done:
		default:
			close(f.done)
		}
	}
}

type collectorActor struct {
	fleet *Fleet
}

func (c *collectorActor) Receive(ctx context.Context, msg Message) error {
	if msg.Type != MsgTypeResult {
		return nil
	}

	payload, ok := msg.Payload.(ResultPayload)
	if !ok {
		return fmt.Errorf("invalid result payload")
	}

	c.fleet.addResult(FleetResult{
		WorkerName: msg.From,
		Result:     payload.Result,
		Error:      payload.Error,
		Data:       payload.Data,
	})

	return nil
}
