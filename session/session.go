// Package session runs the agentic inference loop: send a message, call tools,
// feed results back, repeat until the model stops requesting tool calls.
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

// Session holds the in-memory state of an ongoing conversation: the agent
// driving it, the working directory for tool execution, and the message
// history. Sessions are ephemeral — callers that want persistence must save
// History() and replay it into a new Session (which is exactly what the HTTP
// server does).
type Session struct {
	id      string
	workDir string
	agent   *agent.Agent
	history []core.Message
	mu      sync.RWMutex
}

// Option is a functional option for New.
type Option func(*Session)

// New creates a Session and generates a ULID id.
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

// WithWorkDir sets the working directory used for tool execution.
func WithWorkDir(dir string) Option {
	return func(s *Session) { s.workDir = dir }
}

// WithAgent sets the agent that drives inference.
func WithAgent(a *agent.Agent) Option {
	return func(s *Session) { s.agent = a }
}

// ============================================================
//  Getters / setters
// ============================================================

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

// History returns a copy of the conversation history.
func (s *Session) History() []core.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]core.Message, len(s.history))
	copy(result, s.history)
	return result
}

// AddMessage appends a message to the history. Used when replaying a stored
// conversation before calling Run.
func (s *Session) AddMessage(msg core.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msg)
}

// Clear empties the conversation history.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []core.Message{}
}

// ============================================================
//  Result types
// ============================================================

// Result is returned by Run when the agentic loop completes.
type Result struct {
	Response  string           // final text response from the model
	ToolCalls []ToolCallResult // all tool calls made across all steps
	Usage     core.Usage       // token usage summed across all steps
	Steps     int              // number of inference calls made
}

// ToolCallResult records the outcome of one tool invocation.
type ToolCallResult struct {
	ToolName string // the tool's Name() — the human-readable tool name
	Input    any
	Output   string
	Error    error
}

// ============================================================
//  Sentinel errors
// ============================================================

var (
	ErrNoProvider = fmt.Errorf("agent has no provider configured")
	ErrNoAgent    = fmt.Errorf("agent is required")
)

// ============================================================
//  Run — blocking agentic loop
// ============================================================

// Run sends message to the model and runs the agentic loop until the model
// produces a final response (no more tool calls). The conversation history is
// updated in place.
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
				ToolUseID: result.ToolName, // ToolName == call.ID, used for tool_use/tool_result pairing
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

// ============================================================
//  Tool execution
// ============================================================

// executeToolCalls runs each tool call sequentially and returns results.
//
// ToolCallResult.ToolName is the call ID (e.g. "toulu_abc123") — this is what
// gets stored in the tool_result ContentBlock's ToolUseID field so the model
// can pair it with the original tool_use block. The human-readable tool name
// is available via call.Name but is not separately stored on ToolCallResult
// to avoid confusion.
func (s *Session) executeToolCalls(ctx context.Context, calls []core.ContentBlock, registry *tool.Registry, workDir string) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))

	for i, call := range calls {
		// ToolName stores the call ID for tool_result ToolUseID pairing.
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
