package protocols

import (
	"encoding/json"
	"fmt"
	"regexp"

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
	legacy := legacyGeminiModel.MatchString(info.ID)
	for _, msg := range replayMessages(info, req.Messages) {
		role := "user"
		if msg.Role == models.RoleAssistant {
			role = "model"
		}
		parts := []any{}
		attachments := []any{}
		for _, part := range msg.Content {
			switch p := part.(type) {
			case models.TextPart:
				block := map[string]any{"text": p.Text}
				addGeminiSignature(block, p.ProviderMetadata)
				parts = append(parts, block)
			case models.ImagePart:
				parts = append(parts, geminiImage(p))
			case models.ReasoningPart:
				parts = append(parts, map[string]any{"text": p.Reasoning, "thought": true, "thoughtSignature": p.Encrypted})
			case models.ToolCallPart:
				call := map[string]any{"name": p.Name, "args": p.Input}
				if !legacy {
					call["id"] = p.CallID
				}
				block := map[string]any{"functionCall": call}
				addGeminiSignature(block, p.ProviderMetadata)
				if !legacy && !sameReplayModel(info, msg.Origin) {
					block["thoughtSignature"] = "skip_thought_signature_validator"
				}
				parts = append(parts, block)
			case models.ToolResultPart:
				result, images := splitToolImages(p)
				p = result
				for _, image := range images {
					switch image := image.(type) {
					case models.TextPart:
						attachments = append(attachments, map[string]any{"text": image.Text})
					case models.ImagePart:
						attachments = append(attachments, geminiImage(image))
					}
				}
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
				resultPart := map[string]any{"name": name, "response": response}
				if !legacy {
					resultPart["id"] = p.CallID
				}
				parts = append(parts, map[string]any{"functionResponse": resultPart})
			}
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
		if len(attachments) > 0 {
			contents = append(contents, map[string]any{"role": "user", "parts": attachments})
		}
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
					ID   string         `json:"id"`
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
		var metadata models.Meta
		if part.Signature != "" {
			metadata = models.Meta{"google": map[string]any{"thought_signature": part.Signature}}
		}
		if call := part.FunctionCall; call != nil {
			if call.Args == nil {
				return p.parts, p.invalid("invalid Gemini function arguments", nil)
			}
			id := call.ID
			if id == "" {
				for n := len(p.tools) + 1; ; n++ {
					id = fmt.Sprintf("call_%d", n)
					if p.blockByID["tool:"+id] == nil {
						break
					}
				}
			}
			p.tool(models.ToolCallPart{CallID: id, Name: call.Name, Input: call.Args, ProviderMetadata: metadata}, encodeJSON(call.Args), false)
		} else if part.Text != "" || part.Signature != "" {
			kind := "text"
			if part.Thought {
				kind = "reasoning"
			}
			// Signed parts must be replayed separately from adjacent parts.
			if part.Signature != "" && len(p.blocks) > 0 {
				last := p.blocks[len(p.blocks)-1]
				if last.kind != "tool" {
					p.endBlock(last.kind, last.id)
				}
			}
			id := p.segment(kind)
			if part.Thought {
				p.reasoningDelta(id, part.Text)
				if metadata != nil {
					p.reasoningState(id, part.Signature, metadata)
				}
			} else {
				p.textDelta(id, part.Text)
				if metadata != nil {
					p.block("text", id).metadata = metadata
					p.emit(models.TextDeltaPart{ID: id, ProviderMetadata: metadata})
				}
			}
			if part.Signature != "" {
				p.endBlock(kind, id)
			}
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

func addGeminiSignature(block map[string]any, metadata models.Meta) {
	google, _ := metadata["google"].(map[string]any)
	if signature, _ := google["thought_signature"].(string); signature != "" {
		block["thoughtSignature"] = signature
	}
}

func geminiImage(image models.ImagePart) map[string]any {
	if image.Base64 != "" {
		return map[string]any{"inlineData": map[string]any{"mimeType": imageMediaType(image), "data": image.Base64}}
	}
	return map[string]any{"fileData": map[string]any{"mimeType": imageMediaType(image), "fileUri": image.URL}}
}

// Gemini 1.x/2.x do not support explicit function-call IDs.
var legacyGeminiModel = regexp.MustCompile(`(?i)(^|/)gemini(-live)?-[12]([.-]|$)`)
