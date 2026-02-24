package session

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/tool"
)

type SessionStream struct {
	session      *Session
	ctx          context.Context
	events       chan core.StreamEvent
	result       *Result
	err          error
	toolRegistry *tool.Registry
	workDir      string

	currentEvent   core.StreamEvent
	providerStream core.Stream
	done           bool
}

func (s *Session) RunStream(ctx context.Context, message string) (*SessionStream, error) {
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

	toolRegistry := tool.NewRegistry()
	for _, t := range s.agent.Tools() {
		toolRegistry.Register(t)
	}

	ss := &SessionStream{
		session:      s,
		ctx:          ctx,
		events:       make(chan core.StreamEvent, 100),
		toolRegistry: toolRegistry,
		workDir:      workDir,
		result: &Result{
			ToolCalls: []ToolCallResult{},
		},
	}

	go ss.run(p)

	return ss, nil
}

func (ss *SessionStream) run(p core.Provider) {
	defer close(ss.events)

	for {
		ss.result.Steps++

		ss.session.mu.RLock()
		req := core.InferenceRequest{
			Messages:     ss.session.history,
			Tools:        ss.toolRegistry.Definitions(),
			Instructions: ss.session.agent.Instructions(),
			OutputSchema: ss.session.agent.OutputSchema(),
		}
		ss.session.mu.RUnlock()

		stream, err := p.StreamInference(ss.ctx, req)
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
		ss.session.history = append(ss.session.history, core.Message{
			Role:    core.RoleAssistant,
			Content: resp.Content,
		})
		ss.session.mu.Unlock()

		if !resp.HasToolCalls() {
			ss.result.Response = resp.GetText()
			return
		}

		toolResults := ss.session.executeToolCalls(ss.ctx, resp.GetToolCalls(), ss.toolRegistry, ss.workDir)
		ss.result.ToolCalls = append(ss.result.ToolCalls, toolResults...)

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

		ss.session.mu.Lock()
		ss.session.history = append(ss.session.history, core.Message{
			Role:    core.RoleUser,
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

func (ss *SessionStream) Event() core.StreamEvent {
	return ss.currentEvent
}

func (ss *SessionStream) Err() error {
	return ss.err
}

func (ss *SessionStream) Result() *Result {
	return ss.result
}
