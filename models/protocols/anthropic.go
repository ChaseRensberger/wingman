package protocols

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

// Anthropic implements the Messages protocol.
type Anthropic struct{}

// ID returns the protocol identifier.
func (Anthropic) ID() string { return "anthropic_messages" }

// LoweredOptions reports protocol-calculated request flags.
func (Anthropic) LoweredOptions(models.Request) models.LoweredOptions { return models.LoweredOptions{} }

// Body converts a common request into a Messages request.
func (Anthropic) Body(info models.ModelInfo, req models.Request) (map[string]any, error) {
	messages := []any{}
	for _, msg := range req.Messages {
		role := string(msg.Role)
		content := anthropicBlocks(msg.Content)
		if msg.Role == models.RoleTool {
			role, content = "user", []any{}
			for _, result := range toolResults(msg.Content) {
				blocks := anthropicBlocks(result.Output)
				if result.Structured != nil {
					blocks = append(blocks, map[string]any{"type": "text", "text": encodeJSON(result.Structured)})
				}
				var output any = blocks
				if len(blocks) == 1 {
					block := blocks[0].(map[string]any)
					if block["type"] == "text" {
						output = block["text"]
					}
				}
				block := map[string]any{"type": "tool_result", "tool_use_id": result.CallID, "content": output}
				if result.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
		}
		messages = append(messages, map[string]any{"role": role, "content": content})
	}
	fallback := info.MaxOutput
	if fallback == 0 {
		fallback = 4096
	}
	body := map[string]any{"model": info.ID, "messages": messages, "stream": true, "max_tokens": maxOutput(req, fallback)}
	if req.System != "" {
		body["system"] = []any{map[string]any{"type": "text", "text": req.System}}
	}
	if len(req.Tools) > 0 && req.ToolChoice != models.ToolChoiceNone {
		tools := []any{}
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
		}
		body["tools"] = tools
		switch req.ToolChoice {
		case "", models.ToolChoiceAuto:
		case models.ToolChoiceRequired:
			body["tool_choice"] = map[string]any{"type": "any"}
		default:
			body["tool_choice"] = map[string]any{"type": "tool", "name": string(req.ToolChoice)}
		}
	}
	if req.Capabilities.Thinking {
		body["thinking"] = map[string]any{"type": "adaptive"}
	}
	addGeneration(body, req, "max_tokens")
	return body, nil
}
func anthropicBlocks(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case models.ImagePart:
			if p.Base64 != "" {
				out = append(out, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": imageMediaType(p), "data": p.Base64}})
			}
		case models.ReasoningPart:
			out = append(out, map[string]any{"type": "thinking", "thinking": p.Reasoning, "signature": p.Encrypted})
		case models.ToolCallPart:
			out = append(out, map[string]any{"type": "tool_use", "id": p.CallID, "name": p.Name, "input": p.Input})
		}
	}
	return out
}

// NewParser creates isolated state for one Messages stream.
func (Anthropic) NewParser(info models.ModelInfo) route.Parser {
	return &anthropicParser{state: state{info: info}, calls: map[int]*anthropicCall{}}
}

type anthropicParser struct {
	state
	calls       map[int]*anthropicCall
	reason      models.FinishReason
	usageNative anthropicUsage
}
type anthropicCall struct {
	id, name string
	input    map[string]any
	args     strings.Builder
}
type anthropicUsage struct {
	Input      *int `json:"input_tokens"`
	Output     *int `json:"output_tokens"`
	Cached     *int `json:"cache_read_input_tokens"`
	CacheWrite *int `json:"cache_creation_input_tokens"`
}
type anthropicEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Error *struct {
		Type string `json:"type"`
	} `json:"error"`
	Message struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage        anthropicUsage `json:"usage"`
	ContentBlock struct {
		Type     string         `json:"type"`
		ID       string         `json:"id"`
		Name     string         `json:"name"`
		Input    map[string]any `json:"input"`
		Text     string         `json:"text"`
		Thinking string         `json:"thinking"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
}

func (p *anthropicParser) updateUsage(u anthropicUsage) {
	if u.Input != nil {
		p.usageNative.Input = u.Input
	}
	if u.Output != nil {
		p.usageNative.Output = u.Output
	}
	if u.Cached != nil {
		p.usageNative.Cached = u.Cached
	}
	if u.CacheWrite != nil {
		p.usageNative.CacheWrite = u.CacheWrite
	}
	value := func(n *int) int {
		if n == nil {
			return 0
		}
		return *n
	}
	u = p.usageNative
	p.usage = models.Usage{InputTokens: value(u.Input) + value(u.Cached) + value(u.CacheWrite), OutputTokens: value(u.Output), CachedInputTokens: value(u.Cached), CacheWriteTokens: value(u.CacheWrite)}
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
}
func (p *anthropicParser) Step(frame route.Frame) ([]models.StreamPart, error) {
	p.parts = nil
	var event anthropicEvent
	if err := p.decode(frame.Data, &event); err != nil {
		return nil, err
	}
	if event.Type == "error" || event.Error != nil {
		failure := &models.ProviderError{Provider: p.info.Provider, Category: models.ErrorProvider, Message: "provider response failed", Cause: errors.New(frame.Data)}
		if event.Error != nil {
			switch event.Error.Type {
			case "authentication_error":
				failure.Category, failure.Message = models.ErrorAuthentication, "provider authentication failed"
			case "permission_error":
				failure.Category, failure.Message = models.ErrorAuthorization, "provider access denied"
			case "invalid_request_error", "not_found_error", "request_too_large":
				failure.Category, failure.Message = models.ErrorInvalidRequest, "provider rejected the request"
			case "rate_limit_error":
				failure.Category, failure.Retryable, failure.Message = models.ErrorRateLimit, true, "provider rate limit exceeded"
			case "api_error", "overloaded_error":
				failure.Category, failure.Retryable, failure.Message = models.ErrorUnavailable, true, "provider is unavailable"
			}
		}
		return nil, failure
	}
	switch event.Type {
	case "message_start":
		p.updateUsage(event.Message.Usage)
	case "content_block_start":
		block := event.ContentBlock
		p.textDelta(fmt.Sprintf("text-%d", event.Index), block.Text)
		p.reasoningDelta(fmt.Sprintf("reasoning-%d", event.Index), block.Thinking)
		if block.Type == "tool_use" {
			p.calls[event.Index] = &anthropicCall{id: block.ID, name: block.Name, input: block.Input}
			p.emit(models.ToolInputStartPart{ID: block.ID, ToolName: block.Name})
		}
	case "content_block_delta":
		p.textDelta(fmt.Sprintf("text-%d", event.Index), event.Delta.Text)
		p.reasoningDelta(fmt.Sprintf("reasoning-%d", event.Index), event.Delta.Thinking)
		if event.Delta.Signature != "" {
			p.encrypted += event.Delta.Signature
		}
		if event.Delta.Type == "input_json_delta" {
			acc := p.calls[event.Index]
			if acc == nil {
				return p.parts, p.invalid("tool argument delta without a tool block", nil)
			}
			acc.args.WriteString(event.Delta.PartialJSON)
			p.emit(models.ToolInputDeltaPart{ID: acc.id, Delta: event.Delta.PartialJSON})
		}
	case "content_block_stop":
		if acc := p.calls[event.Index]; acc != nil {
			input := acc.input
			if acc.args.Len() > 0 || input == nil {
				var err error
				input, err = decodeArgs(acc.args.String())
				if err != nil {
					return p.parts, p.invalid("invalid tool arguments", err)
				}
			}
			p.tool(models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: input}, acc.args.String(), true)
			delete(p.calls, event.Index)
		}
	case "message_delta":
		p.updateUsage(event.Usage)
		switch event.Delta.StopReason {
		case "":
		case "end_turn", "stop_sequence", "pause_turn":
			p.reason = models.FinishReasonStop
		case "max_tokens":
			p.reason = models.FinishReasonMaxTokens
		case "tool_use":
			p.reason = models.FinishReasonToolCalls
		case "refusal":
			p.reason = models.FinishReasonBlocked
		default:
			return p.parts, p.invalid("invalid Messages stop reason", nil)
		}
	case "message_stop":
		if len(p.calls) > 0 {
			return p.parts, p.invalid("incomplete tool call at end of stream", nil)
		}
		reason := p.reason
		if reason == "" {
			reason = models.FinishReasonStop
		}
		p.complete(reason)
	}
	return p.parts, nil
}
