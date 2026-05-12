// Package anthropicmessages implements Anthropic's Messages protocol.
package anthropicmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols/internal/shared"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/models/transform"
)

const betaInterleavedThinking = "interleaved-thinking-2025-05-14"

// Protocol adapts models.Request to Anthropic Messages requests and streams.
type Protocol struct{}

func (Protocol) API() models.API { return models.APIAnthropicMessages }

func (Protocol) Prepare(_ context.Context, ref route.ModelRef, req models.Request) (*route.PreparedBody, error) {
	req.Messages = transform.Apply(req.Messages, transform.Target{Provider: ref.Provider, API: models.APIAnthropicMessages, ModelID: ref.ModelID, Capabilities: ref.Info.Capabilities})
	built := buildRequest(ref, req)
	body, err := json.Marshal(built.wire)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal anthropic request: %w", ref.Provider, err)
	}
	headers := make(http.Header)
	headers.Set("anthropic-version", "2023-06-01")
	if built.needsThinkingBeta {
		headers.Set("anthropic-beta", betaInterleavedThinking)
	}
	return &route.PreparedBody{Body: body, Headers: headers}, nil
}

func (Protocol) ParseStream(ctx context.Context, ref route.ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message]) {
	p := newParser(out, shared.Origin(ref.Provider, models.APIAnthropicMessages, ref.ModelID), ref.Provider)
	shared.ScanSSE(ctx, resp, p.handle, p.terminateError, ref.Provider+": stream closed without message_stop")
}

func (Protocol) CountTokens(_ context.Context, _ route.ModelRef, msgs []models.Message) (int, error) {
	return shared.CountTokens(msgs), nil
}

type request struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	Tools        []toolDefinition   `json:"tools,omitempty"`
	ToolChoice   *toolChoice        `json:"tool_choice,omitempty"`
	Thinking     *thinkingConfig    `json:"thinking,omitempty"`
	Stream       bool               `json:"stream,omitempty"`
	OutputConfig *outputConfig      `json:"output_config,omitempty"`
}

type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Signature string         `json:"signature,omitempty"`
	Data      string         `json:"data,omitempty"`
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

type outputConfig struct {
	Format *outputFormat `json:"format,omitempty"`
}

type outputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type builtRequest struct {
	wire              request
	needsThinkingBeta bool
}

func buildRequest(ref route.ModelRef, req models.Request) builtRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = ref.MaxOutputTokens
	}
	r := request{Model: ref.ModelID, MaxTokens: maxOut, System: req.System, Messages: toWireMessages(req.Messages), Stream: true}
	if len(req.Tools) > 0 {
		r.Tools = make([]toolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i] = toolDefinition{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
		}
	}
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case models.ToolChoiceRequired:
			r.ToolChoice = &toolChoice{Type: "any"}
		case models.ToolChoiceNone:
			r.ToolChoice = &toolChoice{Type: "none"}
		case models.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = &toolChoice{Type: "tool", Name: req.ToolChoice.Tool}
			}
		}
	}
	var needsBeta bool
	if th := req.Capabilities.Thinking; th != nil {
		if supportsAdaptiveThinking(ref.ModelID) {
			r.Thinking = &thinkingConfig{Type: "adaptive", Display: "summarized"}
		} else {
			budget := th.BudgetTokens
			if budget <= 0 {
				budget = 1024
			}
			r.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: budget}
			needsBeta = true
			if r.MaxTokens <= budget {
				r.MaxTokens = budget + 1024
			}
		}
	}
	if req.OutputSchema != nil {
		r.OutputConfig = &outputConfig{Format: &outputFormat{Type: "json_schema", Schema: req.OutputSchema.Schema}}
	}
	return builtRequest{wire: r, needsThinkingBeta: needsBeta}
}

func supportsAdaptiveThinking(modelID string) bool {
	return strings.HasPrefix(modelID, "claude-opus-4") || strings.HasPrefix(modelID, "claude-sonnet-4-5") || strings.HasPrefix(modelID, "claude-sonnet-4-6")
}

func toWireMessages(msgs []models.Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		role := string(m.Role)
		if m.Role == models.RoleTool {
			role = "user"
		}
		var blocks []contentBlock
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				if strings.TrimSpace(v.Text) != "" {
					blocks = append(blocks, contentBlock{Type: "text", Text: v.Text})
				}
			case models.ReasoningPart:
				if v.Redacted {
					blocks = append(blocks, contentBlock{Type: "redacted_thinking", Data: v.Signature})
				} else {
					blocks = append(blocks, contentBlock{Type: "thinking", Thinking: v.Reasoning, Signature: v.Signature})
				}
			case models.ToolCallPart:
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: v.CallID, Name: v.Name, Input: v.Input})
			case models.ToolResultPart:
				blocks = append(blocks, contentBlock{Type: "tool_result", ToolUseID: v.CallID, Content: shared.ToolResultText(v.Output), IsError: v.IsError})
			}
		}
		if len(blocks) > 0 {
			out = append(out, anthropicMessage{Role: role, Content: blocks})
		}
	}
	return out
}

type parser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	terminated bool
	origin     *models.MessageOrigin
	provider   string
	blocks     map[int]*blockState
	content    models.Content
	usage      models.Usage
	stopReason string
	meta       models.ResponseMetadata
}

type blockState struct {
	kind      string
	id        string
	toolName  string
	textBuf   strings.Builder
	jsonBuf   strings.Builder
	signature strings.Builder
	redacted  bool
}

func newParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, provider string) *parser {
	return &parser{out: out, origin: origin, provider: provider, blocks: map[int]*blockState{}}
}

func (p *parser) handle(eventType, data string) bool {
	switch eventType {
	case "message_start":
		var ev struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(err)
			return true
		}
		p.meta = models.ResponseMetadata{ID: ev.Message.ID, ModelID: ev.Message.Model}
		p.usage.InputTokens = ev.Message.Usage.InputTokens
		p.usage.OutputTokens = ev.Message.Usage.OutputTokens
		p.out.Push(models.StreamStartPart{})
		p.out.Push(models.ResponseMetadataPart{ResponseMetadata: p.meta})
	case "content_block_start":
		var ev struct {
			Index        int                             `json:"index"`
			ContentBlock struct{ Type, ID, Name string } `json:"content_block"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return false
		}
		id := "blk_" + strconv.Itoa(ev.Index)
		switch ev.ContentBlock.Type {
		case "text":
			p.blocks[ev.Index] = &blockState{kind: "text", id: id}
			p.out.Push(models.TextStartPart{ID: id})
		case "thinking", "redacted_thinking":
			p.blocks[ev.Index] = &blockState{kind: "thinking", id: id, redacted: ev.ContentBlock.Type == "redacted_thinking"}
			p.out.Push(models.ReasoningStartPart{ID: id})
		case "tool_use":
			p.blocks[ev.Index] = &blockState{kind: "tool", id: ev.ContentBlock.ID, toolName: ev.ContentBlock.Name}
			p.out.Push(models.ToolInputStartPart{ID: ev.ContentBlock.ID, ToolName: ev.ContentBlock.Name})
		}
	case "content_block_delta":
		var ev struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil || p.blocks[ev.Index] == nil {
			return false
		}
		b := p.blocks[ev.Index]
		switch ev.Delta.Type {
		case "text_delta":
			b.textBuf.WriteString(ev.Delta.Text)
			p.out.Push(models.TextDeltaPart{ID: b.id, Delta: ev.Delta.Text})
		case "thinking_delta":
			b.textBuf.WriteString(ev.Delta.Thinking)
			p.out.Push(models.ReasoningDeltaPart{ID: b.id, Delta: ev.Delta.Thinking})
		case "signature_delta":
			b.signature.WriteString(ev.Delta.Signature)
		case "input_json_delta":
			b.jsonBuf.WriteString(ev.Delta.PartialJSON)
			p.out.Push(models.ToolInputDeltaPart{ID: b.id, Delta: ev.Delta.PartialJSON})
		}
	case "content_block_stop":
		var ev struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		p.finishBlock(ev.Index)
	case "message_delta":
		var ev struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		p.stopReason = ev.Delta.StopReason
		p.usage.OutputTokens = ev.Usage.OutputTokens
	case "message_stop":
		p.terminate(mapStopReason(p.stopReason), nil)
		return true
	case "error":
		p.terminate(models.FinishReasonError, fmt.Errorf("%s: stream error", p.provider))
		return true
	}
	return false
}

func (p *parser) finishBlock(index int) {
	b := p.blocks[index]
	if b == nil {
		return
	}
	switch b.kind {
	case "text":
		p.content = append(p.content, models.TextPart{Text: b.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: b.id})
	case "thinking":
		p.content = append(p.content, models.ReasoningPart{Reasoning: b.textBuf.String(), Signature: b.signature.String(), Redacted: b.redacted})
		p.out.Push(models.ReasoningEndPart{ID: b.id})
	case "tool":
		var input map[string]any
		_ = json.Unmarshal([]byte(b.jsonBuf.String()), &input)
		p.content = append(p.content, models.ToolCallPart{CallID: b.id, Name: b.toolName, Input: input})
		p.out.Push(models.ToolInputEndPart{ID: b.id})
		p.out.Push(models.ToolCallPart_{ID: b.id, ToolName: b.toolName, Input: input})
	}
	delete(p.blocks, index)
}

func mapStopReason(r string) models.FinishReason {
	switch r {
	case "end_turn", "stop_sequence", "pause_turn":
		return models.FinishReasonStop
	case "max_tokens":
		return models.FinishReasonLength
	case "tool_use":
		return models.FinishReasonToolCalls
	case "refusal":
		return models.FinishReasonContentFilter
	case "":
		return models.FinishReasonUnknown
	default:
		return models.FinishReasonOther
	}
}

func (p *parser) terminateError(err error) { p.terminate(models.FinishReasonError, err) }

func (p *parser) terminate(reason models.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.terminated = true
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
	msg := &models.Message{Role: models.RoleAssistant, Content: p.content, Origin: p.origin, FinishReason: reason}
	if err != nil {
		p.out.Push(models.ErrorPart{Message: err.Error()})
	}
	p.out.Push(models.FinishPart{Reason: reason, Usage: p.usage, Message: msg, Metadata: p.meta})
	p.out.Close(msg, err)
}

var _ route.Protocol = Protocol{}
