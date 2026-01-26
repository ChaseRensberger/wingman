package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"wingman/models"
	"wingman/provider"
	"wingman/session"
	"wingman/tool"
)

type Agent struct {
	name         string
	instructions string
	provider     provider.Provider
	session      *session.Session
	tools        *tool.Registry
	maxTokens    int
	temperature  *float64
	maxSteps     int
}

type AgentOption func(*Agent) error

func New(name string, p provider.Provider, opts ...AgentOption) (*Agent, error) {
	a := &Agent{
		name:     name,
		provider: p,
		session:  session.New(),
		tools:    tool.NewRegistry(),
		maxSteps: 50,
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return a, nil
}

func WithInstructions(instructions string) AgentOption {
	return func(a *Agent) error {
		a.instructions = instructions
		return nil
	}
}

func WithMaxTokens(maxTokens int) AgentOption {
	return func(a *Agent) error {
		a.maxTokens = maxTokens
		return nil
	}
}

func WithTemperature(temperature float64) AgentOption {
	return func(a *Agent) error {
		a.temperature = &temperature
		return nil
	}
}

func WithMaxSteps(maxSteps int) AgentOption {
	return func(a *Agent) error {
		a.maxSteps = maxSteps
		return nil
	}
}

func WithTools(tools ...tool.Tool) AgentOption {
	return func(a *Agent) error {
		for _, t := range tools {
			a.tools.Register(t)
		}
		return nil
	}
}

type RunResult struct {
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

type AgentEventType string

const (
	EventToken      AgentEventType = "token"
	EventToolCall   AgentEventType = "tool_call"
	EventToolResult AgentEventType = "tool_result"
	EventUsage      AgentEventType = "usage"
	EventDone       AgentEventType = "done"
	EventError      AgentEventType = "error"
)

type AgentEvent struct {
	Type       AgentEventType
	Content    string
	ToolCall   *ToolCallResult
	ToolResult *ToolCallResult
	Usage      *models.WingmanUsage
	Result     *RunResult
	Error      error
}

func (a *Agent) Run(ctx context.Context, prompt string) (*RunResult, error) {
	a.session.AddMessage(models.NewUserMessage(prompt))

	var totalUsage models.WingmanUsage
	var allToolCalls []ToolCallResult
	steps := 0

	for {
		if a.maxSteps > 0 && steps >= a.maxSteps {
			return nil, fmt.Errorf("max steps (%d) exceeded", a.maxSteps)
		}
		steps++

		req := models.WingmanInferenceRequest{
			Messages:     a.session.Messages(),
			Tools:        a.tools.Definitions(),
			MaxTokens:    a.maxTokens,
			Temperature:  a.temperature,
			Instructions: a.instructions,
		}

		resp, err := a.provider.RunInference(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("inference failed: %w", err)
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		a.session.AddMessage(models.WingmanMessage{
			Role:    models.RoleAssistant,
			Content: resp.Content,
		})

		if !resp.HasToolCalls() {
			return &RunResult{
				Response:  resp.GetText(),
				ToolCalls: allToolCalls,
				Usage:     totalUsage,
				Steps:     steps,
			}, nil
		}

		toolResults := a.executeToolCalls(ctx, resp.GetToolCalls())
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

		a.session.AddMessage(models.WingmanMessage{
			Role:    models.RoleUser,
			Content: resultBlocks,
		})
	}
}

func (a *Agent) RunStream(ctx context.Context, prompt string) (<-chan AgentEvent, error) {
	events := make(chan AgentEvent, 100)

	go a.runStreamLoop(ctx, prompt, events)

	return events, nil
}

func (a *Agent) runStreamLoop(ctx context.Context, prompt string, events chan<- AgentEvent) {
	defer close(events)

	a.session.AddMessage(models.NewUserMessage(prompt))

	var totalUsage models.WingmanUsage
	var allToolCalls []ToolCallResult
	steps := 0

	for {
		if a.maxSteps > 0 && steps >= a.maxSteps {
			events <- AgentEvent{
				Type:  EventError,
				Error: fmt.Errorf("max steps (%d) exceeded", a.maxSteps),
			}
			return
		}
		steps++

		req := models.WingmanInferenceRequest{
			Messages:     a.session.Messages(),
			Tools:        a.tools.Definitions(),
			MaxTokens:    a.maxTokens,
			Temperature:  a.temperature,
			Instructions: a.instructions,
		}

		streamCh, err := a.provider.StreamInference(ctx, req)
		if err != nil {
			events <- AgentEvent{
				Type:  EventError,
				Error: fmt.Errorf("stream inference failed: %w", err),
			}
			return
		}

		var respContent []models.WingmanContentBlock
		var stopReason string
		var pendingToolCalls []models.WingmanContentBlock

		for streamEvent := range streamCh {
			select {
			case <-ctx.Done():
				events <- AgentEvent{
					Type:  EventError,
					Error: ctx.Err(),
				}
				return
			default:
			}

			switch streamEvent.Type {
			case provider.StreamEventToken:
				events <- AgentEvent{
					Type:    EventToken,
					Content: streamEvent.Content,
				}

			case provider.StreamEventToolCall:
				if block, ok := streamEvent.Delta.(models.WingmanContentBlock); ok {
					pendingToolCalls = append(pendingToolCalls, block)
					events <- AgentEvent{
						Type: EventToolCall,
						ToolCall: &ToolCallResult{
							ToolName: block.ID,
							Input:    block.Input,
						},
					}
				}

			case provider.StreamEventUsage:
				if streamEvent.Usage != nil {
					totalUsage.InputTokens += streamEvent.Usage.InputTokens
					totalUsage.OutputTokens += streamEvent.Usage.OutputTokens
				}

			case provider.StreamEventDone:
				if resp, ok := streamEvent.Delta.(*models.WingmanInferenceResponse); ok {
					respContent = resp.Content
					stopReason = resp.StopReason
				}

			case provider.StreamEventError:
				events <- AgentEvent{
					Type:  EventError,
					Error: streamEvent.Error,
				}
				return
			}
		}

		a.session.AddMessage(models.WingmanMessage{
			Role:    models.RoleAssistant,
			Content: respContent,
		})

		if stopReason != "tool_use" {
			text := ""
			for _, block := range respContent {
				if block.Type == models.ContentTypeText {
					text = block.Text
					break
				}
			}
			events <- AgentEvent{
				Type:  EventUsage,
				Usage: &totalUsage,
			}
			events <- AgentEvent{
				Type: EventDone,
				Result: &RunResult{
					Response:  text,
					ToolCalls: allToolCalls,
					Usage:     totalUsage,
					Steps:     steps,
				},
			}
			return
		}

		toolResults := a.executeToolCalls(ctx, pendingToolCalls)
		allToolCalls = append(allToolCalls, toolResults...)

		var resultBlocks []models.WingmanContentBlock
		for _, result := range toolResults {
			events <- AgentEvent{
				Type:       EventToolResult,
				ToolResult: &result,
			}

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

		a.session.AddMessage(models.WingmanMessage{
			Role:    models.RoleUser,
			Content: resultBlocks,
		})
	}
}

func (a *Agent) executeToolCalls(ctx context.Context, calls []models.WingmanContentBlock) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))

	for i, call := range calls {
		results[i] = ToolCallResult{
			ToolName: call.ID,
			Input:    call.Input,
		}

		t, err := a.tools.Get(call.Name)
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

func (a *Agent) Session() *session.Session {
	return a.session
}

func (a *Agent) Name() string {
	return a.name
}
