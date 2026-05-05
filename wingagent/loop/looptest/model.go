// Package looptest provides deterministic test helpers for wingagent/loop.
// RecordingModel scripts pre-canned responses; RecordingSink captures emitted
// events. Intended for use only in tests.
package looptest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/wingmodels"
)

// ScriptedReply describes one pre-canned model response.
type ScriptedReply struct {
	text      string
	toolCalls []scriptedToolCall
	err       error
	usage     wingmodels.Usage
}

type scriptedToolCall struct {
	name string
	args map[string]any
}

// Reply returns a ScriptedReply that emits the given assistant text.
func Reply(text string) ScriptedReply { return ScriptedReply{text: text} }

// ReplyWithTool returns a ScriptedReply that emits a single tool call with
// the given name and JSON-encoded arguments.
func ReplyWithTool(name, jsonArgs string) ScriptedReply {
	var args map[string]any
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		panic(fmt.Sprintf("looptest.ReplyWithTool: invalid JSON args: %v", err))
	}
	return ScriptedReply{toolCalls: []scriptedToolCall{{name: name, args: args}}}
}

// ToolCall is a helper for building multi-tool ScriptedReplies.
type ToolCall struct{ Name string; Args map[string]any }

// ReplyWithToolCalls returns a ScriptedReply that emits multiple tool calls.
func ReplyWithToolCalls(calls ...ToolCall) ScriptedReply {
	tcs := make([]scriptedToolCall, len(calls))
	for i, c := range calls {
		tcs[i] = scriptedToolCall{name: c.Name, args: deepCopy(c.Args)}
	}
	return ScriptedReply{toolCalls: tcs}
}

// ReplyError returns a ScriptedReply that simulates a provider error on
// Stream setup.
func ReplyError(err error) ScriptedReply { return ScriptedReply{err: err} }

// RecordingModel implements wingmodels.Model by replaying a script of
// pre-canned replies. It records every incoming request for later inspection.
type RecordingModel struct {
	mu       sync.Mutex
	script   []ScriptedReply
	index    int
	requests []RecordedRequest
}

// NewRecordingModel constructs a RecordingModel with the given script.
func NewRecordingModel(script ...ScriptedReply) *RecordingModel { return &RecordingModel{script: script} }

// RecordedRequest captures the arguments to a single Stream call.
type RecordedRequest struct {
	System          string
	Messages        []wingmodels.Message
	Tools           []wingmodels.ToolDef
	ToolChoice      wingmodels.ToolChoice
	Capabilities    wingmodels.Capabilities
	OutputSchema    *wingmodels.OutputSchema
	MaxOutputTokens int
}

// Requests returns a copy of the recorded requests.
func (m *RecordingModel) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return deepCopy(m.requests)
}

// Info returns static metadata about the model.
func (m *RecordingModel) Info() wingmodels.ModelInfo {
	return wingmodels.ModelInfo{Provider: "looptest", ID: "recording-model",
		Capabilities: wingmodels.ModelCapabilities{Tools: true}}
}

// Stream replays the next scripted reply as a synthetic event stream.
func (m *RecordingModel) Stream(ctx context.Context, req wingmodels.Request) (*wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, RecordedRequest{
		System: req.System, Messages: deepCopy(req.Messages), Tools: deepCopy(req.Tools),
		ToolChoice: req.ToolChoice, Capabilities: req.Capabilities,
		OutputSchema: req.OutputSchema, MaxOutputTokens: req.MaxOutputTokens,
	})
	m.index++
	if m.index > len(m.script) {
		return nil, fmt.Errorf("looptest: script exhausted, request %d has no scripted reply", m.index)
	}
	reply := m.script[m.index-1]
	if reply.err != nil {
		return nil, reply.err
	}
	stream := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](64)
	go func() {
		msg := assembleMessage(reply)
		defer stream.Close(msg, nil)
		stream.Push(wingmodels.StreamStartPart{})
		if reply.text != "" {
			stream.Push(wingmodels.TextStartPart{ID: "t0"})
			stream.Push(wingmodels.TextDeltaPart{ID: "t0", Delta: reply.text})
			stream.Push(wingmodels.TextEndPart{ID: "t0"})
		}
		for i, tc := range reply.toolCalls {
			id := fmt.Sprintf("c%d", i)
			b, _ := json.Marshal(tc.args)
			stream.Push(wingmodels.ToolInputStartPart{ID: id, ToolName: tc.name})
			stream.Push(wingmodels.ToolInputDeltaPart{ID: id, Delta: string(b)})
			stream.Push(wingmodels.ToolInputEndPart{ID: id})
			stream.Push(wingmodels.ToolCallPart_{ID: id, ToolName: tc.name, Input: tc.args})
		}
		reason := wingmodels.FinishReasonStop
		if len(reply.toolCalls) > 0 {
			reason = wingmodels.FinishReasonToolCalls
		}
		stream.Push(wingmodels.FinishPart{Reason: reason, Usage: reply.usage, Message: msg})
	}()
	return stream, nil
}

// CountTokens returns a dummy token count.
func (m *RecordingModel) CountTokens(ctx context.Context, msgs []wingmodels.Message) (int, error) {
	total := 0
	for _, msg := range msgs {
		for _, p := range msg.Content {
			if t, ok := p.(wingmodels.TextPart); ok {
				total += len(t.Text)
			}
		}
	}
	return total / 4, nil
}

func assembleMessage(reply ScriptedReply) *wingmodels.Message {
	var content wingmodels.Content
	if reply.text != "" {
		content = append(content, wingmodels.TextPart{Text: reply.text})
	}
	for i, tc := range reply.toolCalls {
		content = append(content, wingmodels.ToolCallPart{CallID: fmt.Sprintf("c%d", i), Name: tc.name, Input: tc.args})
	}
	reason := wingmodels.FinishReasonStop
	if len(reply.toolCalls) > 0 {
		reason = wingmodels.FinishReasonToolCalls
	}
	return &wingmodels.Message{
		Role: wingmodels.RoleAssistant, Content: content, FinishReason: reason,
		Origin: &wingmodels.MessageOrigin{Provider: "looptest", API: wingmodels.APIOpenAICompletions, ModelID: "recording-model"},
	}
}

func deepCopy[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("looptest: deep copy: %v", err))
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(fmt.Sprintf("looptest: deep copy: %v", err))
	}
	return out
}
