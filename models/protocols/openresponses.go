package protocols

import (
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

// Responses implements the reusable Responses wire protocol.
type Responses struct{}

// ID returns the protocol identifier.
func (Responses) ID() string { return "openai_responses" }

// LoweredOptions reports automatic reasoning summaries.
func (Responses) LoweredOptions(req models.Request) models.LoweredOptions {
	return models.LoweredOptions{ReasoningSummaryAuto: req.Capabilities.Thinking}
}

// Body converts a common request into a Responses request.
func (Responses) Body(info models.ModelInfo, req models.Request) (map[string]any, error) {
	input := make([]any, 0, len(req.Messages)+1)
	if req.System != "" {
		input = append(input, map[string]any{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			content := []any{}
			for _, part := range msg.Content {
				switch p := part.(type) {
				case models.TextPart:
					content = append(content, map[string]any{"type": "input_text", "text": p.Text})
				case models.ImagePart:
					if url := imageURL(p); url != "" {
						content = append(content, map[string]any{"type": "input_image", "image_url": url})
					}
				}
			}
			input = append(input, map[string]any{"role": "user", "content": content})
		case models.RoleAssistant:
			texts := []any{}
			flushText := func() {
				if len(texts) > 0 {
					input = append(input, map[string]any{"role": "assistant", "content": texts})
					texts = nil
				}
			}
			for _, part := range msg.Content {
				switch p := part.(type) {
				case models.TextPart:
					texts = append(texts, map[string]any{"type": "output_text", "text": p.Text})
				case models.ReasoningPart:
					flushText()
					metadata, _ := p.ProviderMetadata["openai"].(map[string]any)
					if id, _ := metadata["item_id"].(string); id == "" || p.Encrypted == "" || metadata["reasoning_encrypted_content"] != p.Encrypted {
						continue
					}
					summary := []any{}
					if p.Reasoning != "" {
						summary = append(summary, map[string]any{"type": "summary_text", "text": p.Reasoning})
					}
					input = append(input, map[string]any{"type": "reasoning", "summary": summary, "encrypted_content": p.Encrypted})
				case models.ToolCallPart:
					flushText()
					input = append(input, map[string]any{"type": "function_call", "call_id": p.CallID, "name": p.Name, "arguments": encodeJSON(p.Input)})
				}
			}
			flushText()
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				input = append(input, map[string]any{"type": "function_call_output", "call_id": result.CallID, "output": toolResultText(result)})
			}
		}
	}
	body := map[string]any{"model": info.ID, "input": input, "stream": true}
	if req.Capabilities.Thinking {
		body["reasoning"] = map[string]any{"summary": "auto"}
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	if len(req.Tools) > 0 {
		tools := []any{}
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema})
		}
		body["tools"] = tools
	}
	switch req.ToolChoice {
	case "", models.ToolChoiceAuto:
	case models.ToolChoiceNone, models.ToolChoiceRequired:
		body["tool_choice"] = string(req.ToolChoice)
	default:
		body["tool_choice"] = map[string]any{"type": "function", "name": string(req.ToolChoice)}
	}
	format := outputFormat(req)
	if format.Type != "" {
		value := map[string]any{"type": format.Type}
		if format.Type == "json_schema" {
			value["name"], value["schema"], value["strict"] = defaultName(format.Name), format.Schema, format.Strict
		}
		body["text"] = map[string]any{"format": value}
	}
	addGeneration(body, req, "max_output_tokens")
	return body, nil
}

// NewParser creates isolated state for one Responses stream.
func (Responses) NewParser(info models.ModelInfo) route.Parser {
	return &responsesParser{state: state{info: info}, arguments: map[string]*strings.Builder{}, calls: map[string]string{}}
}

type responsesParser struct {
	state
	arguments map[string]*strings.Builder
	calls     map[string]string
}
type responsesEvent struct {
	Type         string `json:"type"`
	Delta        string `json:"delta"`
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Part         struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"part"`
	Item struct {
		Type             string `json:"type"`
		ID               string `json:"id"`
		CallID           string `json:"call_id"`
		Name             string `json:"name"`
		Arguments        string `json:"arguments"`
		EncryptedContent string `json:"encrypted_content"`
	} `json:"item"`
	Response struct {
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
			InputDetails struct {
				Cached int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails struct {
				Reasoning int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
}

func (p *responsesParser) Step(frame route.Frame) ([]models.StreamPart, error) {
	p.parts = nil
	if frame.Data == "[DONE]" {
		return nil, nil
	}
	var event responsesEvent
	if err := p.decode(frame.Data, &event); err != nil {
		return nil, err
	}
	switch event.Type {
	case "response.failed", "error":
		return nil, models.ClassifyProviderFailure(p.info.Provider, 0, frame.Data)
	case "response.output_item.added":
		switch item := event.Item; item.Type {
		case "reasoning":
			p.block("reasoning", responsesReasoningID(item.ID))
		case "function_call":
			p.calls[item.ID] = item.CallID
			p.arguments[item.ID] = &strings.Builder{}
			p.toolStart(item.CallID, item.Name, nil)
		}
	case "response.content_part.added":
		if event.Part.Type == "output_text" {
			id := responsesTextID(event.ItemID, event.ContentIndex)
			p.block("text", id)
			p.textDelta(id, event.Part.Text)
		}
	case "response.output_text.delta":
		p.textDelta(responsesTextID(event.ItemID, event.ContentIndex), event.Delta)
	case "response.content_part.done":
		if event.Part.Type == "output_text" {
			p.endBlock("text", responsesTextID(event.ItemID, event.ContentIndex))
		}
	case "response.reasoning_text.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta":
		p.reasoningDelta(responsesReasoningID(event.ItemID), event.Delta)
	case "response.function_call_arguments.delta":
		if event.ItemID == "" {
			break
		}
		if p.arguments[event.ItemID] == nil {
			p.arguments[event.ItemID] = &strings.Builder{}
		}
		p.arguments[event.ItemID].WriteString(event.Delta)
		if callID, ok := p.calls[event.ItemID]; ok {
			p.toolDelta(callID, event.Delta)
		}
	case "response.output_item.done":
		item := event.Item
		if item.Type == "reasoning" {
			id := responsesReasoningID(item.ID)
			p.reasoningState(id, item.EncryptedContent, models.Meta{"openai": map[string]any{"item_id": item.ID, "reasoning_encrypted_content": item.EncryptedContent}})
			p.endBlock("reasoning", id)
		}
		if item.Type == "function_call" {
			raw := item.Arguments
			if raw == "" && p.arguments[item.ID] != nil {
				raw = p.arguments[item.ID].String()
			}
			input, err := decodeArgs(raw)
			if err != nil {
				return p.parts, p.invalid("invalid tool arguments", err)
			}
			_, started := p.calls[item.ID]
			if started && p.arguments[item.ID].Len() == 0 && raw != "" {
				p.toolDelta(item.CallID, raw)
			}
			p.tool(models.ToolCallPart{CallID: item.CallID, Name: item.Name, Input: input}, raw, started)
			delete(p.arguments, item.ID)
			delete(p.calls, item.ID)
		}
	case "response.completed", "response.incomplete":
		usage := event.Response.Usage
		p.usage = models.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens, CachedInputTokens: usage.InputDetails.Cached, ReasoningTokens: usage.OutputDetails.Reasoning}
		if len(p.arguments) > 0 {
			return p.parts, p.invalid("incomplete tool call at end of stream", nil)
		}
		reason := models.FinishReasonStop
		if event.Type == "response.incomplete" {
			reason = models.FinishReasonError
		}
		switch event.Response.IncompleteDetails.Reason {
		case "max_output_tokens":
			reason = models.FinishReasonMaxTokens
		case "content_filter":
			reason = models.FinishReasonBlocked
		}
		if len(p.tools) > 0 && reason == models.FinishReasonStop {
			reason = models.FinishReasonToolCalls
		}
		p.complete(reason)
	}
	return p.parts, nil
}

func responsesTextID(itemID string, index int) string {
	return fmt.Sprintf("text-%s-%d", itemID, index)
}

func responsesReasoningID(itemID string) string {
	if itemID == "" {
		return "reasoning-0"
	}
	return itemID
}
