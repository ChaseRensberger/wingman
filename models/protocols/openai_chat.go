package protocols

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

// Chat implements the OpenAI-compatible Chat Completions protocol.
type Chat struct{}

// ID returns the protocol identifier.
func (Chat) ID() string { return "openai_chat" }

// LoweredOptions reports protocol-calculated request flags.
func (Chat) LoweredOptions(models.Request) models.LoweredOptions { return models.LoweredOptions{} }

// Body converts a common request into a Chat Completions request.
func (Chat) Body(info models.ModelInfo, req models.Request) (map[string]any, error) {
	messages := []any{}
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			var content any = joinText(msg.Content)
			parts := []any{}
			image := false
			for _, part := range msg.Content {
				switch p := part.(type) {
				case models.TextPart:
					parts = append(parts, map[string]any{"type": "text", "text": p.Text})
				case models.ImagePart:
					image = true
					if url := imageURL(p); url != "" {
						parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
					}
				}
			}
			if image {
				content = parts
			}
			messages = append(messages, map[string]any{"role": "user", "content": content})
		case models.RoleAssistant:
			message := map[string]any{"role": "assistant", "content": joinText(msg.Content)}
			calls := []any{}
			for _, call := range toolCalls(msg.Content) {
				calls = append(calls, map[string]any{"id": call.CallID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": encodeJSON(call.Input)}})
			}
			if len(calls) > 0 {
				message["tool_calls"] = calls
			}
			messages = append(messages, message)
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": result.CallID, "content": toolResultText(result)})
			}
		}
	}
	body := map[string]any{"model": info.ID, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(req.Tools) > 0 {
		tools := []any{}
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
		}
		body["tools"] = tools
	}
	switch req.ToolChoice {
	case "", models.ToolChoiceAuto:
	case models.ToolChoiceNone, models.ToolChoiceRequired:
		body["tool_choice"] = string(req.ToolChoice)
	default:
		body["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": string(req.ToolChoice)}}
	}
	format := outputFormat(req)
	if format.Type != "" {
		value := map[string]any{"type": format.Type}
		if format.Type == "json_schema" {
			value["json_schema"] = map[string]any{"name": defaultName(format.Name), "schema": format.Schema, "strict": format.Strict}
		}
		body["response_format"] = value
	}
	addGeneration(body, req, "max_tokens")
	return body, nil
}

// NewParser creates isolated state for one Chat stream.
func (Chat) NewParser(info models.ModelInfo) route.Parser {
	return &chatParser{state: state{info: info}, calls: map[int]*chatCall{}}
}

type chatParser struct {
	state
	calls  map[int]*chatCall
	reason models.FinishReason
}
type chatCall struct {
	id, name string
	args     strings.Builder
	started  bool
}
type chatUsage struct {
	Prompt        int `json:"prompt_tokens"`
	Completion    int `json:"completion_tokens"`
	Total         int `json:"total_tokens"`
	PromptDetails struct {
		Cached int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		Reasoning int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u chatUsage) usage() models.Usage {
	return models.Usage{InputTokens: u.Prompt, OutputTokens: u.Completion, TotalTokens: u.Total, CachedInputTokens: u.PromptDetails.Cached, ReasoningTokens: u.CompletionDetails.Reasoning}
}

type chatEvent struct {
	Error   *json.RawMessage `json:"error"`
	Usage   *chatUsage       `json:"usage"`
	Choices []struct {
		FinishReason string     `json:"finish_reason"`
		Usage        *chatUsage `json:"usage"`
		Delta        struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning_content"`
			ToolCalls []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p *chatParser) Step(frame route.Frame) ([]models.StreamPart, error) {
	p.parts = nil
	if frame.Data == "[DONE]" {
		if len(p.calls) > 0 {
			return nil, p.invalid("incomplete tool call at end of stream", nil)
		}
		reason := p.reason
		if reason == "" {
			return nil, p.invalid("Chat stream ended without finish_reason", nil)
		}
		p.complete(reason)
		return p.parts, nil
	}
	var event chatEvent
	if err := p.decode(frame.Data, &event); err != nil {
		return nil, err
	}
	if event.Error != nil {
		return nil, models.ClassifyProviderFailure(p.info.Provider, 0, frame.Data)
	}
	if len(event.Choices) > 0 {
		choice := event.Choices[0]
		if choice.Usage != nil && !choice.Usage.usage().Empty() {
			p.usage = choice.Usage.usage()
		}
		p.textDelta("text-0", choice.Delta.Content)
		p.reasoningDelta("reasoning-0", choice.Delta.Reasoning)
		for _, call := range choice.Delta.ToolCalls {
			if call.Index == nil || *call.Index < 0 {
				return p.parts, p.invalid("invalid Chat tool call index", nil)
			}
			acc := p.calls[*call.Index]
			if acc == nil {
				acc = &chatCall{}
				p.calls[*call.Index] = acc
			}
			if call.ID != "" {
				acc.id = call.ID
			}
			if call.Function.Name != "" {
				acc.name = call.Function.Name
			}
			if fragment := call.Function.Arguments; fragment != "" {
				if !acc.started {
					p.emit(models.ToolInputStartPart{ID: acc.id, ToolName: acc.name})
					acc.started = true
				}
				acc.args.WriteString(fragment)
				p.emit(models.ToolInputDeltaPart{ID: acc.id, Delta: fragment})
			}
		}
		if choice.FinishReason != "" {
			for index := 0; len(p.calls) > 0; index++ {
				acc := p.calls[index]
				if acc == nil {
					return p.parts, p.invalid("non-contiguous Chat tool call indices", nil)
				}
				input, err := decodeArgs(acc.args.String())
				if err != nil {
					return p.parts, p.invalid("invalid tool arguments", err)
				}
				p.tool(models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: input}, acc.args.String(), acc.started)
				delete(p.calls, index)
			}
			switch choice.FinishReason {
			case "stop":
				p.reason = models.FinishReasonStop
			case "length":
				p.reason = models.FinishReasonMaxTokens
			case "tool_calls", "function_call":
				p.reason = models.FinishReasonToolCalls
			case "content_filter":
				p.reason = models.FinishReasonBlocked
			default:
				return p.parts, p.invalid("invalid Chat finish reason", fmt.Errorf("finish reason %q", choice.FinishReason))
			}
			if len(p.tools) > 0 && p.reason == models.FinishReasonStop {
				p.reason = models.FinishReasonToolCalls
			}
		}
	}
	if event.Usage != nil && !event.Usage.usage().Empty() {
		p.usage = event.Usage.usage()
	}
	return p.parts, nil
}

func (p *chatParser) End() ([]models.StreamPart, error) {
	p.parts = nil
	// Compatible endpoints can end after finish_reason; usage may arrive after that frame.
	if p.reason != "" && len(p.calls) == 0 {
		p.complete(p.reason)
	}
	return p.parts, nil
}
