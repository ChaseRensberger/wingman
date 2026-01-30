package actor

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type Message struct {
	ID        string
	From      string
	Type      string
	Payload   any
	Timestamp time.Time
}

func NewMessage(from, msgType string, payload any) Message {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	return Message{
		ID:        id.String(),
		From:      from,
		Type:      msgType,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

type Actor interface {
	Receive(ctx context.Context, msg Message) error
}

type Ref struct {
	name   string
	system *System
}

func (r *Ref) Name() string {
	return r.name
}

func (r *Ref) Send(msg Message) error {
	return r.system.Send(r.name, msg)
}

type actorState struct {
	actor   Actor
	mailbox chan Message
	done    chan struct{}
	err     error
	result  any
	running bool
	mu      sync.Mutex
}

type System struct {
	actors map[string]*actorState
	mu     sync.RWMutex
	wg     sync.WaitGroup
}

func NewSystem() *System {
	return &System{
		actors: make(map[string]*actorState),
	}
}

type SpawnOption func(*spawnConfig)

type spawnConfig struct {
	mailboxSize int
}

func WithMailboxSize(size int) SpawnOption {
	return func(cfg *spawnConfig) {
		cfg.mailboxSize = size
	}
}

func (s *System) Spawn(name string, actor Actor, opts ...SpawnOption) *Ref {
	cfg := &spawnConfig{
		mailboxSize: 100,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	state := &actorState{
		actor:   actor,
		mailbox: make(chan Message, cfg.mailboxSize),
		done:    make(chan struct{}),
		running: true,
	}

	s.mu.Lock()
	s.actors[name] = state
	s.mu.Unlock()

	s.wg.Add(1)
	go s.runActor(name, state)

	return &Ref{name: name, system: s}
}

func (s *System) runActor(name string, state *actorState) {
	defer s.wg.Done()

	ctx := context.Background()

	for {
		select {
		case msg, ok := <-state.mailbox:
			if !ok {
				return
			}
			if err := state.actor.Receive(ctx, msg); err != nil {
				state.mu.Lock()
				state.err = err
				state.running = false
				state.mu.Unlock()
				return
			}
		case <-state.done:
			return
		}
	}
}

func (s *System) Send(name string, msg Message) error {
	s.mu.RLock()
	state, ok := s.actors[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("actor not found: %s", name)
	}

	state.mu.Lock()
	if !state.running {
		state.mu.Unlock()
		return fmt.Errorf("actor %s is not running", name)
	}
	state.mu.Unlock()

	select {
	case state.mailbox <- msg:
		return nil
	default:
		return fmt.Errorf("mailbox full for actor: %s", name)
	}
}

func (s *System) Stop(name string) error {
	s.mu.Lock()
	state, ok := s.actors[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("actor not found: %s", name)
	}
	s.mu.Unlock()

	close(state.done)
	return nil
}

func (s *System) Shutdown() {
	s.mu.Lock()
	for _, state := range s.actors {
		close(state.done)
	}
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *System) Wait() {
	s.wg.Wait()
}

type ActorResult struct {
	Name   string
	Result any
	Error  error
}

func (s *System) Results() []ActorResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]ActorResult, 0, len(s.actors))
	for name, state := range s.actors {
		state.mu.Lock()
		results = append(results, ActorResult{
			Name:   name,
			Result: state.result,
			Error:  state.err,
		})
		state.mu.Unlock()
	}
	return results
}

func (s *System) IsRunning(name string) bool {
	s.mu.RLock()
	state, ok := s.actors[name]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	return state.running
}

func (s *System) Error(name string) error {
	s.mu.RLock()
	state, ok := s.actors[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("actor not found: %s", name)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	return state.err
}
