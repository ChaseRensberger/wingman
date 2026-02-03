package session

import (
	"context"
	"fmt"

	"wingman/models"
	"wingman/provider"
	"wingman/tool"
)

type SessionStream struct {
	session      *Session
	ctx          context.Context
	events       chan models.StreamEvent
	result       *Result
	err          error
	toolRegistry *tool.Registry
	workDir      string

	currentEvent   models.StreamEvent
	providerStream provider.Stream
	done           bool
}

func (s *Session) RunStream(ctx context.Context, prompt string) (*SessionStream, error) {
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
	workDir := s.workDir
	p := s.provider
	s.mu.Unlock()

	toolRegistry := tool.NewRegistry()
	for _, t := range s.agent.Tools() {
		toolRegistry.Register(t)
	}

	ss := &SessionStream{
		session:      s,
		ctx:          ctx,
		events:       make(chan models.StreamEvent, 100),
		toolRegistry: toolRegistry,
		workDir:      workDir,
		result: &Result{
			ToolCalls: []ToolCallResult{},
		},
	}

	go ss.run(p)

	return ss, nil
}

func (ss *SessionStream) run(p provider.Provider) {
	defer close(ss.events)

	maxSteps := ss.session.agent.MaxSteps()
	if maxSteps <= 0 {
		maxSteps = 50
	}

	for {
		if ss.result.Steps >= maxSteps {
			ss.err = fmt.Errorf("max steps (%d) exceeded", maxSteps)
			return
		}
		ss.result.Steps++

		ss.session.mu.RLock()
		req := models.WingmanInferenceRequest{
			Messages:     ss.session.history,
			Tools:        ss.toolRegistry.Definitions(),
			MaxTokens:    ss.session.agent.MaxTokens(),
			Temperature:  ss.session.agent.Temperature(),
			Instructions: ss.session.agent.Instructions(),
			OutputSchema: ss.session.agent.OutputSchema(),
		}
		ss.session.mu.RUnlock()

		stream, err := p.RunInferenceStream(ss.ctx, req)
		if err != nil {
			ss.err = fmt.Errorf("inference failed: %w", err)
			return
		}

		for stream.Next() {
			event := stream.Event()
			select {
			case ss.events <- event:
			case <-ss.ctx.Done():
				stream.Close()
				ss.err = ss.ctx.Err()
				return
			}
		}

		if err := stream.Err(); err != nil {
			ss.err = err
			stream.Close()
			return
		}

		resp := stream.Response()
		stream.Close()

		ss.result.Usage.InputTokens += resp.Usage.InputTokens
		ss.result.Usage.OutputTokens += resp.Usage.OutputTokens

		ss.session.mu.Lock()
		ss.session.history = append(ss.session.history, models.WingmanMessage{
			Role:    models.RoleAssistant,
			Content: resp.Content,
		})
		ss.session.mu.Unlock()

		if !resp.HasToolCalls() {
			ss.result.Response = resp.GetText()
			return
		}

		toolResults := ss.session.executeToolCalls(ss.ctx, resp.GetToolCalls(), ss.toolRegistry, ss.workDir)
		ss.result.ToolCalls = append(ss.result.ToolCalls, toolResults...)

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

		ss.session.mu.Lock()
		ss.session.history = append(ss.session.history, models.WingmanMessage{
			Role:    models.RoleUser,
			Content: resultBlocks,
		})
		ss.session.mu.Unlock()
	}
}

func (ss *SessionStream) Next() bool {
	if ss.done {
		return false
	}

	event, ok := <-ss.events
	if !ok {
		ss.done = true
		return false
	}

	ss.currentEvent = event
	return true
}

func (ss *SessionStream) Event() models.StreamEvent {
	return ss.currentEvent
}

func (ss *SessionStream) Err() error {
	return ss.err
}

func (ss *SessionStream) Result() *Result {
	return ss.result
}
