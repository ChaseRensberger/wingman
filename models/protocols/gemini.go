package protocols

import (
	"encoding/json"
	"fmt"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

// Gemini implements the streaming generateContent protocol.
type Gemini struct{}

// ID returns the protocol identifier.
func (Gemini) ID() string { return "gemini_generate" }

// LoweredOptions reports protocol-calculated request flags.
func (Gemini) LoweredOptions(models.Request) models.LoweredOptions { return models.LoweredOptions{} }

// Body converts a common request into a generateContent request.
func (Gemini) Body(info models.ModelInfo, req models.Request) (map[string]any, error) {
	contents := []any{}
	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == models.RoleAssistant {
			role = "model"
		}
		parts := []any{}
		for _, part := range msg.Content {
			switch p := part.(type) {
			case models.TextPart:
				parts = append(parts, map[string]any{"text": p.Text})
			case models.ImagePart:
				if p.Base64 != "" {
					parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": imageMediaType(p), "data": p.Base64}})
				}
			case models.ReasoningPart:
				parts = append(parts, map[string]any{"text": p.Reasoning, "thought": true, "thoughtSignature": p.Encrypted})
			case models.ToolCallPart:
				parts = append(parts, map[string]any{"functionCall": map[string]any{"name": p.Name, "args": p.Input}})
			case models.ToolResultPart:
				name := p.Name
				if name == "" {
					name = p.CallID
				}
				response := map[string]any{"output": toolResultText(p)}
				if p.Structured != nil {
					response["structured"] = p.Structured
				}
				if p.IsError {
					response["error"] = true
				}
				parts = append(parts, map[string]any{"functionResponse": map[string]any{"name": name, "response": response}})
			}
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}
	body := map[string]any{"contents": contents}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": req.System}}}
	}
	if len(req.Tools) > 0 && req.ToolChoice != models.ToolChoiceNone {
		declarations := []any{}
		for _, tool := range req.Tools {
			declarations = append(declarations, map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema})
		}
		body["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
		choice := map[string]any{"mode": "ANY"}
		switch req.ToolChoice {
		case "", models.ToolChoiceAuto:
		case models.ToolChoiceRequired:
			body["toolConfig"] = map[string]any{"functionCallingConfig": choice}
		default:
			choice["allowedFunctionNames"] = []string{string(req.ToolChoice)}
			body["toolConfig"] = map[string]any{"functionCallingConfig": choice}
		}
	}
	config := map[string]any{}
	format := outputFormat(req)
	if format.Type == "json_schema" || format.Type == "json" {
		config["responseMimeType"] = "application/json"
		if format.Schema != nil {
			config["responseSchema"] = format.Schema
		}
	}
	if max := maxOutput(req, 0); max != 0 {
		config["maxOutputTokens"] = max
	}
	if req.Generation.Temperature != nil {
		config["temperature"] = *req.Generation.Temperature
	}
	if req.Generation.TopP != nil {
		config["topP"] = *req.Generation.TopP
	}
	if len(req.Generation.Stop) > 0 {
		config["stopSequences"] = req.Generation.Stop
	}
	if len(config) > 0 {
		body["generationConfig"] = config
	}
	return body, nil
}

// NewParser creates isolated state for one generateContent stream.
func (Gemini) NewParser(info models.ModelInfo) route.Parser {
	return &geminiParser{state: state{info: info}}
}

type geminiParser struct{ state }
type geminiEvent struct {
	Error          *json.RawMessage `json:"error"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Usage struct {
		Input     int `json:"promptTokenCount"`
		Output    int `json:"candidatesTokenCount"`
		Total     int `json:"totalTokenCount"`
		Cached    int `json:"cachedContentTokenCount"`
		Reasoning int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Candidates []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text         string `json:"text"`
				Thought      bool   `json:"thought"`
				Signature    string `json:"thoughtSignature"`
				FunctionCall *struct {
					Name string         `json:"name"`
					Args map[string]any `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (p *geminiParser) Step(frame route.Frame) ([]models.StreamPart, error) {
	p.parts = nil
	var event geminiEvent
	if err := p.decode(frame.Data, &event); err != nil {
		return nil, err
	}
	if event.Error != nil {
		return nil, models.ClassifyProviderFailure(p.info.Provider, 0, frame.Data)
	}
	u := event.Usage
	usage := models.Usage{InputTokens: u.Input, OutputTokens: u.Output, TotalTokens: u.Total, CachedInputTokens: u.Cached, ReasoningTokens: u.Reasoning}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if !usage.Empty() {
		p.usage = usage
	}
	if event.PromptFeedback.BlockReason != "" {
		p.complete(models.FinishReasonBlocked)
		return p.parts, nil
	}
	if len(event.Candidates) == 0 {
		return nil, nil
	}
	choice := event.Candidates[0]
	for _, part := range choice.Content.Parts {
		if part.Signature != "" {
			p.encrypted = part.Signature
			p.reasonMetadata = models.Meta{"google": map[string]any{"thought_signature": part.Signature}}
		}
		if part.Thought {
			p.reasoningDelta("reasoning-0", part.Text)
		} else {
			p.textDelta("text-0", part.Text)
		}
		if call := part.FunctionCall; call != nil {
			if call.Args == nil {
				return p.parts, p.invalid("invalid Gemini function arguments", nil)
			}
			p.tool(models.ToolCallPart{CallID: fmt.Sprintf("call_%d", len(p.tools)+1), Name: call.Name, Input: call.Args}, encodeJSON(call.Args), false)
		}
	}
	if choice.FinishReason != "" {
		var reason models.FinishReason
		switch choice.FinishReason {
		case "STOP":
			reason = models.FinishReasonStop
		case "MAX_TOKENS":
			reason = models.FinishReasonMaxTokens
		case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
			reason = models.FinishReasonBlocked
		case "MALFORMED_FUNCTION_CALL", "OTHER", "UNEXPECTED_TOOL_CALL":
			reason = models.FinishReasonError
		default:
			return p.parts, p.invalid("invalid Gemini finish reason", nil)
		}
		if len(p.tools) > 0 && reason == models.FinishReasonStop {
			reason = models.FinishReasonToolCalls
		}
		p.complete(reason)
	}
	return p.parts, nil
}
