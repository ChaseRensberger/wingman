// Package protocols implements native model APIs independently of their deployments.
package protocols

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

type state struct {
	info           models.ModelInfo
	text           strings.Builder
	reason         strings.Builder
	textID         string
	reasonID       string
	encrypted      string
	reasonMetadata models.Meta
	tools          []models.ToolCallPart
	usage          models.Usage
	finish         models.FinishReason
	parts          []models.StreamPart
}

func (s *state) Message() *models.Message {
	content := models.Content{}
	if s.text.Len() > 0 {
		content = append(content, models.TextPart{Text: s.text.String()})
	}
	if s.reason.Len() > 0 || s.encrypted != "" {
		content = append(content, models.ReasoningPart{Reasoning: s.reason.String(), Encrypted: s.encrypted, ProviderMetadata: s.reasonMetadata})
	}
	for _, call := range s.tools {
		content = append(content, call)
	}
	msg := &models.Message{Role: models.RoleAssistant, Content: content, FinishReason: s.finish, Origin: &models.MessageOrigin{Provider: s.info.Provider, API: s.info.API, ModelID: s.info.ID}}
	if !s.usage.Empty() {
		usage := s.usage
		msg.Usage = &usage
	}
	return msg
}

func (s *state) End() ([]models.StreamPart, error) { return nil, nil }
func (s *state) emit(part models.StreamPart)       { s.parts = append(s.parts, part) }
func (s *state) textDelta(id, text string) {
	if text == "" {
		return
	}
	if s.text.Len() == 0 {
		s.textID = id
		s.emit(models.TextStartPart{ID: id})
	}
	s.text.WriteString(text)
	s.emit(models.TextDeltaPart{ID: id, Delta: text})
}
func (s *state) reasoningDelta(id, text string) {
	if text == "" {
		return
	}
	if s.reason.Len() == 0 {
		s.reasonID = id
		s.emit(models.ReasoningStartPart{ID: id})
	}
	s.reason.WriteString(text)
	s.emit(models.ReasoningDeltaPart{ID: id, Delta: text})
}
func (s *state) tool(call models.ToolCallPart, raw string, started bool) {
	if !started {
		s.emit(models.ToolInputStartPart{ID: call.CallID, ToolName: call.Name})
		s.emit(models.ToolInputDeltaPart{ID: call.CallID, Delta: raw})
	}
	s.tools = append(s.tools, call)
	s.emit(models.ToolInputEndPart{ID: call.CallID})
	s.emit(models.ToolCallPart_{ID: call.CallID, ToolName: call.Name, Input: call.Input})
}
func (s *state) complete(reason models.FinishReason) {
	s.finish = reason
	if s.reason.Len() > 0 {
		s.emit(models.ReasoningEndPart{ID: s.reasonID, ProviderMetadata: s.reasonMetadata})
	}
	if s.text.Len() > 0 {
		s.emit(models.TextEndPart{ID: s.textID})
	}
	s.emit(models.FinishPart{Reason: reason, Usage: s.usage, Message: s.Message()})
}
func (s *state) decode(data string, into any) error {
	if err := json.Unmarshal([]byte(data), into); err != nil {
		return s.invalid("invalid provider stream event", err)
	}
	return nil
}
func (s *state) invalid(message string, cause error) error {
	return &models.ProviderError{Provider: s.info.Provider, Category: models.ErrorDecoding, Message: message, Cause: cause}
}
func decodeArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("tool arguments must be a JSON object")
	}
	return input, nil
}
func encodeJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
func joinText(content models.Content) string {
	var out []string
	for _, part := range content {
		if text, ok := part.(models.TextPart); ok {
			out = append(out, text.Text)
		}
	}
	return strings.Join(out, "\n")
}
func toolCalls(content models.Content) []models.ToolCallPart {
	var out []models.ToolCallPart
	for _, part := range content {
		if call, ok := part.(models.ToolCallPart); ok {
			out = append(out, call)
		}
	}
	return out
}
func toolResults(content models.Content) []models.ToolResultPart {
	var out []models.ToolResultPart
	for _, part := range content {
		if result, ok := part.(models.ToolResultPart); ok {
			out = append(out, result)
		}
	}
	return out
}
func toolResultText(part models.ToolResultPart) string {
	text := joinText(part.Output)
	if part.Structured == nil {
		return text
	}
	structured := encodeJSON(part.Structured)
	if text == "" {
		return structured
	}
	return text + "\n" + structured
}
func imageURL(p models.ImagePart) string {
	if p.URL != "" {
		return p.URL
	}
	if p.Base64 == "" {
		return ""
	}
	return "data:" + imageMediaType(p) + ";base64," + p.Base64
}
func imageMediaType(p models.ImagePart) string {
	if p.MediaType != "" {
		return p.MediaType
	}
	return "image/png"
}
func maxOutput(req models.Request, fallback int) int {
	if req.Generation.MaxTokens != 0 {
		return req.Generation.MaxTokens
	}
	if req.MaxOutputTokens != 0 {
		return req.MaxOutputTokens
	}
	return fallback
}
func addGeneration(body map[string]any, req models.Request, maxKey string) {
	if max := maxOutput(req, 0); max != 0 {
		body[maxKey] = max
	}
	if req.Generation.Temperature != nil {
		body["temperature"] = *req.Generation.Temperature
	}
	if req.Generation.TopP != nil {
		body["top_p"] = *req.Generation.TopP
	}
	if len(req.Generation.Stop) > 0 {
		body["stop"] = req.Generation.Stop
	}
}
func outputFormat(req models.Request) models.ResponseFormat {
	if req.OutputSchema != nil {
		return models.ResponseFormat{Type: "json_schema", Name: req.OutputSchema.Name, Schema: req.OutputSchema.Schema, Strict: req.OutputSchema.Strict}
	}
	return req.ResponseFormat
}
func defaultName(name string) string {
	if name != "" {
		return name
	}
	return "output"
}
