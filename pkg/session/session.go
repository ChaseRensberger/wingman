package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"wingman/pkg/models"
	"wingman/pkg/provider"
	"wingman/pkg/tool"
)

type Agent interface {
	Name() string
	Instructions() string
	Tools() []tool.Tool
	MaxTokens() int
	Temperature() *float64
	MaxSteps() int
}

type Session struct {
	id       string
	agent    Agent
	provider provider.Provider
	history  []models.WingmanMessage
	mu       sync.RWMutex
}

type Option func(*Session)

func New(opts ...Option) *Session {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	s := &Session{
		id:      id.String(),
		history: []models.WingmanMessage{},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func WithAgent(a Agent) Option {
	return func(s *Session) {
		s.agent = a
	}
}

func WithProvider(p provider.Provider) Option {
	return func(s *Session) {
		s.provider = p
	}
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) SetAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agent = a
}

func (s *Session) SetProvider(p provider.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = p
}

func (s *Session) Agent() Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agent
}

func (s *Session) Provider() provider.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

func (s *Session) History() []models.WingmanMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.WingmanMessage, len(s.history))
	copy(result, s.history)
	return result
}

func (s *Session) AddMessage(msg models.WingmanMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msg)
}

func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []models.WingmanMessage{}
}

type Result struct {
	Response  string
	ToolCalls []ToolCallResult
	Usage     models.WingmanUsage
	Steps     int
}

type ToolCallResult struct {
	ToolName string
	Input    any
	Output   string
	Error    error
}

var (
	ErrNoProvider = fmt.Errorf("provider is required")
	ErrNoAgent    = fmt.Errorf("agent is required")
)

func (s *Session) Run(ctx context.Context, prompt string) (*Result, error) {
	s.mu.Lock()
	if s.provider == nil {
		s.mu.Unlock()
		return nil, ErrNoProvider
	}
	if s.agent == nil {
		s.mu.Unlock()
		return nil, ErrNoAgent
	}

	s.history = append(s.history, models.NewUserMessage(prompt))
	s.mu.Unlock()

	var totalUsage models.WingmanUsage
	var allToolCalls []ToolCallResult
	steps := 0

	toolRegistry := tool.NewRegistry()
	for _, t := range s.agent.Tools() {
		toolRegistry.Register(t)
	}

	maxSteps := s.agent.MaxSteps()
	if maxSteps <= 0 {
		maxSteps = 50
	}

	for {
		if steps >= maxSteps {
			return nil, fmt.Errorf("max steps (%d) exceeded", maxSteps)
		}
		steps++

		s.mu.RLock()
		req := models.WingmanInferenceRequest{
			Messages:     s.history,
			Tools:        toolRegistry.Definitions(),
			MaxTokens:    s.agent.MaxTokens(),
			Temperature:  s.agent.Temperature(),
			Instructions: s.agent.Instructions(),
		}
		p := s.provider
		s.mu.RUnlock()

		resp, err := p.RunInference(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("inference failed: %w", err)
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		s.mu.Lock()
		s.history = append(s.history, models.WingmanMessage{
			Role:    models.RoleAssistant,
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

		toolResults := s.executeToolCalls(ctx, resp.GetToolCalls(), toolRegistry)
		allToolCalls = append(allToolCalls, toolResults...)

		var resultBlocks []models.WingmanContentBlock
		for _, result := range toolResults {
			content := result.Output
			isError := false
			if result.Error != nil {
				content = result.Error.Error()
				isError = true
			}
			resultBlocks = append(resultBlocks, models.WingmanContentBlock{
				Type:      models.ContentTypeToolResult,
				ToolUseID: result.ToolName,
				Content:   content,
				IsError:   isError,
			})
		}

		s.mu.Lock()
		s.history = append(s.history, models.WingmanMessage{
			Role:    models.RoleUser,
			Content: resultBlocks,
		})
		s.mu.Unlock()
	}
}

func (s *Session) executeToolCalls(ctx context.Context, calls []models.WingmanContentBlock, registry *tool.Registry) []ToolCallResult {
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

		output, err := t.Execute(ctx, params)
		if err != nil {
			results[i].Error = err
			results[i].Output = output
		} else {
			results[i].Output = output
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
