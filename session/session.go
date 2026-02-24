package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/tool"
)

type Session struct {
	id      string
	workDir string
	agent   *agent.Agent
	history []core.Message
	mu      sync.RWMutex
}

type Option func(*Session)

func New(opts ...Option) *Session {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	s := &Session{
		id:      id.String(),
		history: []core.Message{},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func WithWorkDir(dir string) Option {
	return func(s *Session) { s.workDir = dir }
}

func WithAgent(a *agent.Agent) Option {
	return func(s *Session) { s.agent = a }
}

func (s *Session) ID() string { return s.id }

func (s *Session) WorkDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workDir
}

func (s *Session) SetWorkDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workDir = dir
}

func (s *Session) SetAgent(a agent.Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agent = &a
}

func (s *Session) Agent() *agent.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agent
}

func (s *Session) History() []core.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]core.Message, len(s.history))
	copy(result, s.history)
	return result
}

func (s *Session) AddMessage(msg core.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msg)
}

func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []core.Message{}
}

type Result struct {
	Response  string
	ToolCalls []ToolCallResult
	Usage     core.Usage
	Steps     int
}

type ToolCallResult struct {
	ToolName string
	Input    any
	Output   string
	Error    error
}

var (
	ErrNoProvider = fmt.Errorf("agent has no provider configured")
	ErrNoAgent    = fmt.Errorf("agent is required")
)

func (s *Session) Run(ctx context.Context, message string) (*Result, error) {
	s.mu.Lock()
	if s.agent == nil {
		s.mu.Unlock()
		return nil, ErrNoAgent
	}
	if s.agent.Provider() == nil {
		s.mu.Unlock()
		return nil, ErrNoProvider
	}

	s.history = append(s.history, core.NewUserMessage(message))
	workDir := s.workDir
	p := s.agent.Provider()
	s.mu.Unlock()

	var totalUsage core.Usage
	var allToolCalls []ToolCallResult
	steps := 0

	toolRegistry := tool.NewRegistry()
	for _, t := range s.agent.Tools() {
		toolRegistry.Register(t)
	}

	for {
		steps++

		s.mu.RLock()
		req := core.InferenceRequest{
			Messages:     s.history,
			Tools:        toolRegistry.Definitions(),
			Instructions: s.agent.Instructions(),
			OutputSchema: s.agent.OutputSchema(),
		}
		s.mu.RUnlock()

		resp, err := p.RunInference(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("inference failed: %w", err)
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		s.mu.Lock()
		s.history = append(s.history, core.Message{
			Role:    core.RoleAssistant,
			Content: resp.Content,
		})
		s.mu.Unlock()

		if !resp.HasToolCalls() {
			return &Result{
				Response:  resp.GetText(),
				ToolCalls: allToolCalls,
				Usage:     totalUsage,
				Steps:     steps,
			}, nil
		}

		toolResults := s.executeToolCalls(ctx, resp.GetToolCalls(), toolRegistry, workDir)
		allToolCalls = append(allToolCalls, toolResults...)

		var resultBlocks []core.ContentBlock
		for _, result := range toolResults {
			content := result.Output
			isError := false
			if result.Error != nil {
				content = result.Error.Error()
				isError = true
			}
			resultBlocks = append(resultBlocks, core.ContentBlock{
				Type:      core.ContentTypeToolResult,
				ToolUseID: result.ToolName,
				Content:   content,
				IsError:   isError,
			})
		}

		s.mu.Lock()
		s.history = append(s.history, core.Message{
			Role:    core.RoleUser,
			Content: resultBlocks,
		})
		s.mu.Unlock()
	}
}

func (s *Session) executeToolCalls(ctx context.Context, calls []core.ContentBlock, registry *tool.Registry, workDir string) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))

	for i, call := range calls {
		results[i] = ToolCallResult{
			ToolName: call.ID,
			Input:    call.Input,
		}

		t, err := registry.Get(call.Name)
		if err != nil {
			results[i].Error = fmt.Errorf("tool not found: %s", call.Name)
			continue
		}

		params, err := toParamsMap(call.Input)
		if err != nil {
			results[i].Error = fmt.Errorf("invalid tool input: %w", err)
			continue
		}

		output, err := t.Execute(ctx, params, workDir)
		results[i].Output = output
		if err != nil {
			results[i].Error = err
		}
	}

	return results
}

func toParamsMap(input any) (map[string]any, error) {
	if input == nil {
		return map[string]any{}, nil
	}
	if m, ok := input.(map[string]any); ok {
		return m, nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
