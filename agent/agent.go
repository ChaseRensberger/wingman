package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"wingman/models"
	"wingman/provider"
	"wingman/tool"
)

type Agent struct {
	name         string
	instructions string
	tools        []tool.Tool
	maxTokens    int
	temperature  *float64
	maxSteps     int
}

type Option func(*Agent)

func New(name string, opts ...Option) *Agent {
	a := &Agent{
		name:     name,
		maxSteps: 50,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

func WithInstructions(instructions string) Option {
	return func(a *Agent) {
		a.instructions = instructions
	}
}

func WithMaxTokens(maxTokens int) Option {
	return func(a *Agent) {
		a.maxTokens = maxTokens
	}
}

func WithTemperature(temperature float64) Option {
	return func(a *Agent) {
		a.temperature = &temperature
	}
}

func WithMaxSteps(maxSteps int) Option {
	return func(a *Agent) {
		a.maxSteps = maxSteps
	}
}

func WithTools(tools ...tool.Tool) Option {
	return func(a *Agent) {
		a.tools = append(a.tools, tools...)
	}
}

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) Instructions() string {
	return a.instructions
}

func (a *Agent) Tools() []tool.Tool {
	return a.tools
}

func (a *Agent) MaxTokens() int {
	return a.maxTokens
}

func (a *Agent) Temperature() *float64 {
	return a.temperature
}

func (a *Agent) MaxSteps() int {
	return a.maxSteps
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

func (a *Agent) Run(ctx context.Context, p provider.Provider, prompt string) (*Result, error) {
	if p == nil {
		return nil, fmt.Errorf("provider is required")
	}

	s := newEphemeralSession(a, p)
	return s.run(ctx, prompt)
}

type ephemeralSession struct {
	agent    *Agent
	provider provider.Provider
	history  []models.WingmanMessage
}

func newEphemeralSession(a *Agent, p provider.Provider) *ephemeralSession {
	return &ephemeralSession{
		agent:    a,
		provider: p,
		history:  []models.WingmanMessage{},
	}
}

func (s *ephemeralSession) run(ctx context.Context, prompt string) (*Result, error) {
	s.history = append(s.history, models.NewUserMessage(prompt))

	var totalUsage models.WingmanUsage
	var allToolCalls []ToolCallResult
	steps := 0

	toolRegistry := tool.NewRegistry()
	for _, t := range s.agent.tools {
		toolRegistry.Register(t)
	}

	for {
		if s.agent.maxSteps > 0 && steps >= s.agent.maxSteps {
			return nil, fmt.Errorf("max steps (%d) exceeded", s.agent.maxSteps)
		}
		steps++

		req := models.WingmanInferenceRequest{
			Messages:     s.history,
			Tools:        toolRegistry.Definitions(),
			MaxTokens:    s.agent.maxTokens,
			Temperature:  s.agent.temperature,
			Instructions: s.agent.instructions,
		}

		resp, err := s.provider.RunInference(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("inference failed: %w", err)
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		s.history = append(s.history, models.WingmanMessage{
			Role:    models.RoleAssistant,
			Content: resp.Content,
		})

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

		s.history = append(s.history, models.WingmanMessage{
			Role:    models.RoleUser,
			Content: resultBlocks,
		})
	}
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

func (s *ephemeralSession) executeToolCalls(ctx context.Context, calls []models.WingmanContentBlock, registry *tool.Registry) []ToolCallResult {
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
