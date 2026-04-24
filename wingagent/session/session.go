// Package session is a thin stateful wrapper over wingagent/loop.
//
// A Session owns:
//   - an identifier (ULID)
//   - a working directory passed to tool executions
//   - a wingmodels.Model + system prompt + tool registry
//   - the running message history
//
// Session itself is concurrency-safe (mu-guarded). Run and RunStream
// drive a single inference loop turn batch and append both the user
// message and any new assistant/tool messages produced by the loop into
// the session's running history.
//
// Session is deliberately minimal: it owns no persistence, no transport,
// and no compaction. The caller (typically wingagent/server) wires those
// in by reading History() after Run returns.
package session

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
)

// Session is a single conversation. Construct with New.
type Session struct {
	id      string
	workDir string
	model   wingmodels.Model
	system  string
	tools   []tool.Tool

	history []wingmodels.Message
	mu      sync.RWMutex
}

// Option configures a new Session.
type Option func(*Session)

// New returns a Session with a freshly minted ULID and the supplied
// options applied. A new Session has an empty history and no model;
// Run/RunStream will return ErrNoModel until WithModel (or SetModel) is
// applied.
func New(opts ...Option) *Session {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	s := &Session{
		id:      id.String(),
		history: []wingmodels.Message{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithWorkDir sets the working directory tools will see.
func WithWorkDir(dir string) Option {
	return func(s *Session) { s.workDir = dir }
}

// WithModel sets the wingmodels.Model used for inference.
func WithModel(m wingmodels.Model) Option {
	return func(s *Session) { s.model = m }
}

// WithSystem sets the system prompt sent on every turn.
func WithSystem(prompt string) Option {
	return func(s *Session) { s.system = prompt }
}

// WithTools registers the tools the model may call.
func WithTools(tools ...tool.Tool) Option {
	return func(s *Session) { s.tools = append(s.tools, tools...) }
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// WorkDir returns the configured working directory.
func (s *Session) WorkDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workDir
}

// SetWorkDir updates the working directory.
func (s *Session) SetWorkDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workDir = dir
}

// SetModel swaps the active model. Useful for handlers that build the
// model lazily after constructing the session.
func (s *Session) SetModel(m wingmodels.Model) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = m
}

// SetSystem replaces the system prompt.
func (s *Session) SetSystem(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = prompt
}

// SetTools replaces the tool registry.
func (s *Session) SetTools(tools []tool.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = tools
}

// History returns a snapshot copy of the running message history.
func (s *Session) History() []wingmodels.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]wingmodels.Message, len(s.history))
	copy(out, s.history)
	return out
}

// AddMessage appends a message to the history without invoking the
// model. Handlers use this to rehydrate a session from persistent
// storage before calling Run.
func (s *Session) AddMessage(msg wingmodels.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msg)
}

// SetHistory replaces the entire history. The slice is copied; later
// mutations of msgs do not affect the session.
func (s *Session) SetHistory(msgs []wingmodels.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append([]wingmodels.Message(nil), msgs...)
}

// Clear empties the history.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []wingmodels.Message{}
}

// Result is the terminal value of a Run / RunStream invocation.
type Result struct {
	// Response is the concatenated text content of the final assistant
	// message. Empty if the loop terminated without producing a
	// tool-call-free turn.
	Response string

	// ToolCalls is the per-call summary of every tool invocation across
	// every turn of this Run, in execution-completion order.
	ToolCalls []ToolCallResult

	// Usage is the cumulative token usage reported by the provider.
	Usage wingmodels.Usage

	// Steps is the number of assistant turns the loop ran.
	Steps int

	// StopReason tells callers why the loop terminated. Mirrors
	// loop.StopReason exactly; re-exported here so callers don't import
	// the loop package just for the constants.
	StopReason loop.StopReason
}

// ToolCallResult is a serialization-friendly view of one tool call.
// Wire format: handlers JSON-encode this into HTTP responses, so the
// field names matter.
type ToolCallResult struct {
	ToolName string `json:"tool_name"`
	Input    any    `json:"input,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Sentinel errors. ErrNoModel is returned when Run is called before a
// model has been configured.
var (
	ErrNoModel = errors.New("session: no model configured")
)

// Run drives one user message through the loop synchronously.
//
// On return, the session's history contains the input user message plus
// every assistant and tool-result message the loop produced. The returned
// Result is always non-nil even when err is non-nil, so callers can
// persist partial state.
func (s *Session) Run(ctx context.Context, message string) (*Result, error) {
	return s.runWith(ctx, message, nil)
}

// runWith is the shared core for Run and RunStream. extraSink, if
// non-nil, is invoked for every loop event in addition to the session's
// internal sink. The session's own sink collects ToolCallResults and
// keeps the running history in sync.
func (s *Session) runWith(ctx context.Context, message string, extraSink loop.Sink) (*Result, error) {
	s.mu.Lock()
	if s.model == nil {
		s.mu.Unlock()
		return nil, ErrNoModel
	}
	// Append the user message before starting the loop so it ends up in
	// history even if the loop fails immediately.
	s.history = append(s.history, wingmodels.Message{
		Role:    wingmodels.RoleUser,
		Content: wingmodels.Content{wingmodels.TextPart{Text: message}},
	})
	// Snapshot inputs.
	startLen := len(s.history)
	historySnap := append([]wingmodels.Message(nil), s.history...)
	model := s.model
	system := s.system
	tools := s.tools
	workDir := s.workDir
	s.mu.Unlock()

	// Collect tool results in execution order via the sink.
	collected := []ToolCallResult{}
	internal := loop.SinkFunc(func(e loop.Event) {
		if end, ok := e.(loop.ToolExecutionEndEvent); ok {
			collected = append(collected, ToolCallResult{
				ToolName: end.Result.Name,
				Input:    end.Result.Args,
				Output:   end.Result.Output,
				Error:    errStringIf(end.Result.IsError, end.Result.Output),
			})
		}
		if extraSink != nil {
			extraSink.OnEvent(e)
		}
	})

	cfg := loop.Config{
		Model:    model,
		Messages: historySnap,
		System:   system,
		Tools:    tools,
		WorkDir:  workDir,
		Sink:     internal,
	}

	res, runErr := loop.Run(ctx, cfg)

	// Even on error, res is non-nil (loop.Run guarantees it). Adopt the
	// new tail of res.Messages into the session history (everything past
	// the user message we appended).
	s.mu.Lock()
	if res != nil && len(res.Messages) >= startLen {
		s.history = append(s.history[:startLen], res.Messages[startLen:]...)
	}
	s.mu.Unlock()

	out := &Result{
		ToolCalls: collected,
	}
	if res != nil {
		out.Usage = res.Usage
		out.Steps = res.Steps
		out.StopReason = res.StopReason
		// Extract response text from the last assistant message, if any.
		if last := lastAssistant(res.Messages); last != nil {
			out.Response = textOf(*last)
		}
	}
	if runErr != nil {
		return out, fmt.Errorf("loop: %w", runErr)
	}
	return out, nil
}

// errStringIf returns msg when isError is true, "" otherwise. Centralizes
// the contract that ToolCallResult.Error mirrors the IsError flag.
func errStringIf(isError bool, msg string) string {
	if !isError {
		return ""
	}
	return msg
}

// lastAssistant returns a pointer to the last RoleAssistant message in
// msgs, or nil if there is none. Used to extract Result.Response.
func lastAssistant(msgs []wingmodels.Message) *wingmodels.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == wingmodels.RoleAssistant {
			return &msgs[i]
		}
	}
	return nil
}

// textOf concatenates every TextPart in a message in source order.
// Reasoning parts and tool calls are excluded; callers that need the
// full content walk msg.Content directly.
func textOf(msg wingmodels.Message) string {
	var out string
	for _, p := range msg.Content {
		if t, ok := p.(wingmodels.TextPart); ok {
			out += t.Text
		}
	}
	return out
}
