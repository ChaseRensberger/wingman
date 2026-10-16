// Package protocols implements native model APIs independently of their deployments.
package protocols

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

type state struct {
	info      models.ModelInfo
	blocks    []*contentBlock
	blockByID map[string]*contentBlock
	tools     []models.ToolCallPart
	usage     models.Usage
	finish    models.FinishReason
	parts     []models.StreamPart
}

type contentBlock struct {
	id, kind  string
	text      strings.Builder
	encrypted string
	metadata  models.Meta
	call      models.ToolCallPart
	raw       strings.Builder
	closed    bool
}

func (s *state) Message() *models.Message {
	content := models.Content{}
	for _, block := range s.blocks {
		switch block.kind {
		case "text":
			content = append(content, models.TextPart{Text: block.text.String(), ProviderMetadata: block.metadata})
		case "reasoning":
			content = append(content, models.ReasoningPart{Reasoning: block.text.String(), Encrypted: block.encrypted, ProviderMetadata: block.metadata})
		case "tool":
			if block.closed {
				content = append(content, block.call)
			} else {
				content = append(content, models.ToolPart{CallID: block.id, Name: block.call.Name, State: models.ToolStatePending, InputRaw: block.raw.String(), ProviderMetadata: block.metadata})
			}
		}
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

func (s *state) block(kind, id string) *contentBlock {
	key := kind + ":" + id
	if block := s.blockByID[key]; block != nil {
		return block
	}
	if s.blockByID == nil {
		s.blockByID = make(map[string]*contentBlock)
	}
	block := &contentBlock{id: id, kind: kind}
	s.blockByID[key] = block
	s.blocks = append(s.blocks, block)
	switch kind {
	case "text":
		s.emit(models.TextStartPart{ID: id})
	case "reasoning":
		s.emit(models.ReasoningStartPart{ID: id})
	}
	return block
}

func (s *state) textDelta(id, text string) {
	if text == "" {
		return
	}
	s.block("text", id).text.WriteString(text)
	s.emit(models.TextDeltaPart{ID: id, Delta: text})
}
func (s *state) reasoningDelta(id, text string) {
	if text == "" {
		return
	}
	s.block("reasoning", id).text.WriteString(text)
	s.emit(models.ReasoningDeltaPart{ID: id, Delta: text})
}

func (s *state) reasoningState(id, encrypted string, metadata models.Meta) {
	block := s.block("reasoning", id)
	block.encrypted, block.metadata = encrypted, metadata
	s.emit(models.ReasoningDeltaPart{ID: id, Encrypted: encrypted, ProviderMetadata: metadata})
}

// segment joins consecutive chunks when the protocol has no text block IDs.
func (s *state) segment(kind string) string {
	if n := len(s.blocks); n > 0 {
		last := s.blocks[n-1]
		if last.kind == kind && !last.closed {
			return last.id
		}
		if last.kind != "tool" {
			s.endBlock(last.kind, last.id)
		}
	}
	id := fmt.Sprintf("%s-%d", kind, len(s.blocks))
	s.block(kind, id)
	return id
}

func (s *state) toolStart(id, name string, metadata models.Meta) {
	if s.blockByID["tool:"+id] != nil {
		return
	}
	block := s.block("tool", id)
	block.call = models.ToolCallPart{CallID: id, Name: name, ProviderMetadata: metadata}
	block.metadata = metadata
	s.emit(models.ToolInputStartPart{ID: id, ToolName: name, ProviderMetadata: metadata})
}

func (s *state) toolDelta(id, raw string) {
	s.block("tool", id).raw.WriteString(raw)
	s.emit(models.ToolInputDeltaPart{ID: id, Delta: raw})
}

func (s *state) tool(call models.ToolCallPart, raw string, started bool) {
	if !started {
		s.toolStart(call.CallID, call.Name, call.ProviderMetadata)
		s.toolDelta(call.CallID, raw)
	}
	block := s.block("tool", call.CallID)
	block.call, block.closed = call, true
	s.tools = append(s.tools, call)
	s.emit(models.ToolInputEndPart{ID: call.CallID, ProviderMetadata: call.ProviderMetadata})
	s.emit(models.ToolCallPart_{ID: call.CallID, ToolName: call.Name, Input: call.Input, ProviderExecuted: call.ProviderExecuted, ProviderMetadata: call.ProviderMetadata})
}

func (s *state) endBlock(kind, id string) {
	block := s.blockByID[kind+":"+id]
	if block == nil || block.closed {
		return
	}
	block.closed = true
	switch kind {
	case "text":
		s.emit(models.TextEndPart{ID: id, ProviderMetadata: block.metadata})
	case "reasoning":
		s.emit(models.ReasoningEndPart{ID: id, Encrypted: block.encrypted, ProviderMetadata: block.metadata})
	}
}
func (s *state) complete(reason models.FinishReason) {
	s.finish = reason
	for _, block := range s.blocks {
		if block.kind != "tool" {
			s.endBlock(block.kind, block.id)
		}
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
