package httpmodel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// Protocol identifies a supported provider wire protocol.
type Protocol string

const (
	OpenAIResponses   Protocol = "openai_responses"
	OpenAIChat        Protocol = "openai_chat"
	AnthropicMessages Protocol = "anthropic_messages"
)

// Model is a small HTTP/SSE-backed implementation for the supported providers.
type Model struct {
	Info_    models.ModelInfo
	Protocol Protocol
	BaseURL  string
	APIKey   string
	Client   *http.Client
}

// Stream sends a streaming request and parses provider SSE into WingModels parts.
func (m *Model) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	body, err := m.body(req)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", m.Info_.Provider, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	for k, v := range req.HTTP.Headers {
		httpReq.Header.Set(k, v)
	}
	m.auth(httpReq)

	client := m.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("%s stream: HTTP %d: %s", m.Info_.Provider, resp.StatusCode, strings.TrimSpace(string(b)))
	}

	stream := models.NewEventStream[models.StreamPart, *models.Message](64)
	go func() {
		defer resp.Body.Close()
		msg, usage, reason, err := m.readSSE(resp.Body, stream)
		if err != nil {
			stream.Push(models.ErrorPart{Error: err.Error()})
			stream.Close(msg, err)
			return
		}
		stream.Push(models.FinishPart{Reason: reason, Usage: usage, Message: msg})
		stream.Close(msg, nil)
	}()
	return stream, nil
}

// Prepare lowers a provider-neutral request into the provider JSON body without
// sending it.
func (m *Model) Prepare(ctx context.Context, req models.Request) (*models.PreparedRequest, error) {
	body, err := m.body(req)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"content-type": "application/json"}
	for k, v := range req.HTTP.Headers {
		headers[k] = v
	}
	if m.Protocol == AnthropicMessages {
		headers["anthropic-version"] = "2023-06-01"
		headers["anthropic-beta"] = "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
	}
	return &models.PreparedRequest{
		Model: models.ModelRef{
			Provider:      m.Info_.Provider,
			ID:            m.Info_.ID,
			API:           m.Info_.API,
			BaseURL:       m.BaseURL,
			Env:           m.Info_.Env,
			ContextWindow: m.Info_.ContextWindow,
			MaxOutput:     m.Info_.MaxOutput,
			Capabilities:  m.Info_.Capabilities,
		},
		API:     m.Info_.API,
		URL:     m.url(),
		Headers: headers,
		Body:    body,
		Metadata: map[string]any{
			"protocol": string(m.Protocol),
		},
	}, nil
}

// Generate drains Stream and returns the final assistant message.
func (m *Model) Generate(ctx context.Context, req models.Request) (*models.Message, error) {
	return models.Generate(ctx, m, req)
}

// Info returns static model metadata.
func (m *Model) Info() models.ModelInfo { return m.Info_ }

// CountTokens implements a local chars/4 heuristic.
func (m *Model) CountTokens(ctx context.Context, msgs []models.Message) (int, error) {
	total := 0
	for _, msg := range msgs {
		for _, part := range msg.Content {
			if t, ok := part.(models.TextPart); ok {
				total += len(t.Text)
			}
		}
	}
	return total / 4, nil
}

func (m *Model) url() string {
	base := strings.TrimRight(m.BaseURL, "/")
	switch m.Protocol {
	case OpenAIResponses:
		return base + "/responses"
	case OpenAIChat:
		return base + "/chat/completions"
	case AnthropicMessages:
		return base + "/messages"
	default:
		return base
	}
}

func (m *Model) auth(req *http.Request) {
	if m.Protocol == AnthropicMessages {
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14")
		if m.APIKey != "" {
			req.Header.Set("x-api-key", m.APIKey)
		}
		return
	}
	if m.APIKey == "" {
		return
	}
	req.Header.Set("authorization", "Bearer "+m.APIKey)
}

func (m *Model) body(req models.Request) (map[string]any, error) {
	switch m.Protocol {
	case OpenAIResponses:
		return m.openAIResponsesBody(req)
	case OpenAIChat:
		return m.openAIChatBody(req)
	case AnthropicMessages:
		return m.anthropicBody(req)
	default:
		return nil, fmt.Errorf("unsupported protocol %q", m.Protocol)
	}
}

func (m *Model) openAIResponsesBody(req models.Request) (map[string]any, error) {
	input := make([]any, 0, len(req.Messages)+1)
	if req.System != "" {
		input = append(input, map[string]any{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			input = append(input, map[string]any{"role": "user", "content": openAIResponsesTextContent(msg.Content, "input_text")})
		case models.RoleAssistant:
			texts := openAIResponsesTextContent(msg.Content, "output_text")
			if len(texts) > 0 {
				input = append(input, map[string]any{"role": "assistant", "content": texts})
			}
			for _, call := range toolCalls(msg.Content) {
				input = append(input, map[string]any{"type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": encodeJSON(call.Input)})
			}
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				input = append(input, map[string]any{"type": "function_call_output", "call_id": result.CallID, "output": toolResultText(result)})
			}
		}
	}
	body := map[string]any{"model": m.Info_.ID, "input": input, "stream": true}
	addTools(body, req.Tools, "responses")
	addCommonOptions(body, req)
	return overlay(body, req.HTTP.Body), nil
}

func (m *Model) openAIChatBody(req models.Request) (map[string]any, error) {
	messages := make([]any, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, map[string]any{"role": "user", "content": joinText(msg.Content)})
		case models.RoleAssistant:
			m := map[string]any{"role": "assistant", "content": joinText(msg.Content)}
			if calls := toolCalls(msg.Content); len(calls) > 0 {
				arr := make([]any, 0, len(calls))
				for _, call := range calls {
					arr = append(arr, map[string]any{"id": call.CallID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": encodeJSON(call.Input)}})
				}
				m["tool_calls"] = arr
			}
			messages = append(messages, m)
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": result.CallID, "content": toolResultText(result)})
			}
		}
	}
	body := map[string]any{"model": m.Info_.ID, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	addTools(body, req.Tools, "chat")
	addCommonOptions(body, req)
	return overlay(body, req.HTTP.Body), nil
}

func (m *Model) anthropicBody(req models.Request) (map[string]any, error) {
	messages := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, map[string]any{"role": "user", "content": anthropicTextBlocks(msg.Content)})
		case models.RoleAssistant:
			content := anthropicTextBlocks(msg.Content)
			for _, call := range toolCalls(msg.Content) {
				content = append(content, map[string]any{"type": "tool_use", "id": call.CallID, "name": call.Name, "input": call.Input})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": content})
		case models.RoleTool:
			content := make([]any, 0, len(msg.Content))
			for _, result := range toolResults(msg.Content) {
				block := map[string]any{"type": "tool_result", "tool_use_id": result.CallID, "content": toolResultText(result)}
				if result.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
			messages = append(messages, map[string]any{"role": "user", "content": content})
		}
	}
	body := map[string]any{"model": m.Info_.ID, "messages": messages, "stream": true, "max_tokens": maxOutput(req, m.Info_.MaxOutput)}
	if req.System != "" {
		body["system"] = []any{map[string]any{"type": "text", "text": req.System}}
	}
	if len(req.Tools) > 0 && req.ToolChoice != models.ToolChoiceNone {
		tools := make([]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
		}
		body["tools"] = tools
	}
	addCommonOptions(body, req)
	return overlay(body, req.HTTP.Body), nil
}

func (m *Model) readSSE(r io.Reader, stream *models.EventStream[models.StreamPart, *models.Message]) (*models.Message, models.Usage, models.FinishReason, error) {
	stream.Push(models.StreamStartPart{})
	state := parseState{provider: m.Info_.Provider, api: m.Info_.API, model: m.Info_.ID, reason: models.FinishReasonStop}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := m.handleSSEData(data.String(), &state, stream); err != nil {
				return state.message(), state.usage, state.reason, err
			}
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data.Len() > 0 {
		if err := m.handleSSEData(data.String(), &state, stream); err != nil {
			return state.message(), state.usage, state.reason, err
		}
	}
	if err := scanner.Err(); err != nil {
		return state.message(), state.usage, state.reason, err
	}
	return state.message(), state.usage, state.reason, nil
}

func (m *Model) handleSSEData(data string, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	if data == "" || data == "[DONE]" {
		return nil
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return fmt.Errorf("parse %s SSE event: %w", m.Info_.Provider, err)
	}
	switch m.Protocol {
	case OpenAIResponses:
		parseOpenAIResponses(event, state, stream)
	case OpenAIChat:
		parseOpenAIChat(event, state, stream)
	case AnthropicMessages:
		parseAnthropic(event, state, stream)
	}
	return nil
}

type parseState struct {
	provider string
	api      models.API
	model    string
	text     strings.Builder
	tools    []models.ToolCallPart
	usage    models.Usage
	reason   models.FinishReason
	toolBuf  map[string]*toolAccum
}

type toolAccum struct {
	id   string
	name string
	args strings.Builder
}

func (s *parseState) message() *models.Message {
	content := models.Content{}
	if text := s.text.String(); text != "" {
		content = append(content, models.TextPart{Text: text})
	}
	for _, call := range s.tools {
		content = append(content, call)
	}
	return &models.Message{Role: models.RoleAssistant, Content: content, FinishReason: s.reason, Origin: &models.MessageOrigin{Provider: s.provider, API: s.api, ModelID: s.model}}
}

func parseOpenAIResponses(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) {
	typeName, _ := event["type"].(string)
	switch typeName {
	case "response.output_text.delta":
		if delta, _ := event["delta"].(string); delta != "" {
			pushText(state, stream, "text-0", delta)
		}
	case "response.output_item.done":
		item, _ := event["item"].(map[string]any)
		if itemType, _ := item["type"].(string); itemType == "function_call" {
			itemID := stringValue(item["id"])
			arguments := stringValue(item["arguments"])
			if arguments == "" && state.toolBuf != nil {
				if acc := state.toolBuf[itemID]; acc != nil {
					arguments = acc.args.String()
				}
			}
			call := models.ToolCallPart{CallID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Input: decodeArgs(arguments)}
			pushTool(state, stream, call)
		}
	case "response.function_call_arguments.delta":
		itemID := stringValue(event["item_id"])
		if itemID == "" {
			return
		}
		if state.toolBuf == nil {
			state.toolBuf = map[string]*toolAccum{}
		}
		acc := state.toolBuf[itemID]
		if acc == nil {
			acc = &toolAccum{}
			state.toolBuf[itemID] = acc
		}
		acc.args.WriteString(stringValue(event["delta"]))
	case "response.completed", "response.incomplete":
		state.reason = finishReason(stringValue(nested(event, "response", "incomplete_details", "reason")), len(state.tools) > 0)
		state.usage = openAIResponsesUsage(nested(event, "response", "usage"))
	}
}

func parseOpenAIChat(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) {
	choices, _ := event["choices"].([]any)
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if text, _ := delta["content"].(string); text != "" {
			pushText(state, stream, "text-0", text)
		}
		if calls, _ := delta["tool_calls"].([]any); len(calls) > 0 {
			if state.toolBuf == nil {
				state.toolBuf = map[string]*toolAccum{}
			}
			for _, raw := range calls {
				call, _ := raw.(map[string]any)
				idx := fmt.Sprint(intValue(call["index"]))
				acc := state.toolBuf[idx]
				if acc == nil {
					acc = &toolAccum{}
					state.toolBuf[idx] = acc
				}
				if id := stringValue(call["id"]); id != "" {
					acc.id = id
				}
				fn, _ := call["function"].(map[string]any)
				if name := stringValue(fn["name"]); name != "" {
					acc.name = name
				}
				acc.args.WriteString(stringValue(fn["arguments"]))
			}
		}
		if reason := stringValue(choice["finish_reason"]); reason != "" {
			for _, acc := range state.toolBuf {
				pushTool(state, stream, models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: decodeArgs(acc.args.String())})
			}
			state.toolBuf = nil
			state.reason = finishReason(reason, len(state.tools) > 0)
		}
	}
	state.usage = openAIChatUsage(event["usage"])
}

func parseAnthropic(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) {
	typeName := stringValue(event["type"])
	switch typeName {
	case "content_block_start":
		block, _ := event["content_block"].(map[string]any)
		if stringValue(block["type"]) != "tool_use" {
			return
		}
		if state.toolBuf == nil {
			state.toolBuf = map[string]*toolAccum{}
		}
		idx := fmt.Sprint(intValue(event["index"]))
		state.toolBuf[idx] = &toolAccum{id: stringValue(block["id"]), name: stringValue(block["name"])}
		stream.Push(models.ToolInputStartPart{ID: stringValue(block["id"]), ToolName: stringValue(block["name"])})
	case "content_block_delta":
		delta, _ := event["delta"].(map[string]any)
		if text := stringValue(delta["text"]); text != "" {
			pushText(state, stream, fmt.Sprintf("text-%d", intValue(event["index"])), text)
			return
		}
		if stringValue(delta["type"]) == "input_json_delta" {
			idx := fmt.Sprint(intValue(event["index"]))
			if state.toolBuf == nil || state.toolBuf[idx] == nil {
				return
			}
			fragment := stringValue(delta["partial_json"])
			state.toolBuf[idx].args.WriteString(fragment)
			stream.Push(models.ToolInputDeltaPart{ID: state.toolBuf[idx].id, Delta: fragment})
		}
	case "content_block_stop":
		idx := fmt.Sprint(intValue(event["index"]))
		if state.toolBuf == nil || state.toolBuf[idx] == nil {
			return
		}
		acc := state.toolBuf[idx]
		stream.Push(models.ToolInputEndPart{ID: acc.id})
		call := models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: decodeArgs(acc.args.String())}
		state.tools = append(state.tools, call)
		stream.Push(models.ToolCallPart_{ID: call.CallID, ToolName: call.Name, Input: call.Input})
		delete(state.toolBuf, idx)
	case "message_delta":
		delta, _ := event["delta"].(map[string]any)
		state.reason = finishReason(stringValue(delta["stop_reason"]), len(state.tools) > 0)
		state.usage = anthropicUsage(event["usage"])
	}
}

func pushText(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], id, delta string) {
	if state.text.Len() == 0 {
		stream.Push(models.TextStartPart{ID: id})
	}
	state.text.WriteString(delta)
	stream.Push(models.TextDeltaPart{ID: id, Delta: delta})
}

func pushTool(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], call models.ToolCallPart) {
	state.tools = append(state.tools, call)
	stream.Push(models.ToolInputStartPart{ID: call.CallID, ToolName: call.Name})
	stream.Push(models.ToolInputDeltaPart{ID: call.CallID, Delta: encodeJSON(call.Input)})
	stream.Push(models.ToolInputEndPart{ID: call.CallID})
	stream.Push(models.ToolCallPart_{ID: call.CallID, ToolName: call.Name, Input: call.Input})
}

func addTools(body map[string]any, tools []models.ToolDef, mode string) {
	if len(tools) == 0 {
		return
	}
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		if mode == "chat" {
			out = append(out, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
			continue
		}
		out = append(out, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema})
	}
	body["tools"] = out
}

func addCommonOptions(body map[string]any, req models.Request) {
	maxTokens := req.Generation.MaxTokens
	if maxTokens == 0 {
		maxTokens = req.MaxOutputTokens
	}
	if maxTokens != 0 {
		if _, ok := body["max_tokens"]; ok {
			body["max_tokens"] = maxTokens
		} else {
			body["max_output_tokens"] = maxTokens
		}
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

func overlay(base map[string]any, patch map[string]any) map[string]any {
	for k, v := range patch {
		base[k] = v
	}
	return base
}

func maxOutput(req models.Request, fallback int) int {
	if req.Generation.MaxTokens != 0 {
		return req.Generation.MaxTokens
	}
	if req.MaxOutputTokens != 0 {
		return req.MaxOutputTokens
	}
	if fallback != 0 {
		return fallback
	}
	return 4096
}

func openAIResponsesTextContent(content models.Content, typ string) []any {
	out := []any{}
	for _, part := range content {
		if t, ok := part.(models.TextPart); ok {
			out = append(out, map[string]any{"type": typ, "text": t.Text})
		}
	}
	return out
}

func anthropicTextBlocks(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		if t, ok := part.(models.TextPart); ok {
			out = append(out, map[string]any{"type": "text", "text": t.Text})
		}
	}
	return out
}

func joinText(content models.Content) string {
	var out []string
	for _, part := range content {
		if t, ok := part.(models.TextPart); ok {
			out = append(out, t.Text)
		}
	}
	return strings.Join(out, "\n")
}

func toolCalls(content models.Content) []models.ToolCallPart {
	out := []models.ToolCallPart{}
	for _, part := range content {
		if p, ok := part.(models.ToolCallPart); ok {
			out = append(out, p)
		}
	}
	return out
}

func toolResults(content models.Content) []models.ToolResultPart {
	out := []models.ToolResultPart{}
	for _, part := range content {
		if p, ok := part.(models.ToolResultPart); ok {
			out = append(out, p)
		}
	}
	return out
}

func toolResultText(part models.ToolResultPart) string {
	return joinText(part.Output)
}

func encodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeArgs(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func finishReason(raw string, hasTools bool) models.FinishReason {
	if hasTools || raw == "tool_calls" || raw == "function_call" || raw == "tool_use" {
		return models.FinishReasonToolCalls
	}
	if raw == "length" || raw == "max_tokens" || raw == "max_output_tokens" {
		return models.FinishReasonMaxTokens
	}
	return models.FinishReasonStop
}

func openAIChatUsage(v any) models.Usage {
	m, _ := v.(map[string]any)
	return models.Usage{InputTokens: intValue(m["prompt_tokens"]), OutputTokens: intValue(m["completion_tokens"]), TotalTokens: intValue(m["total_tokens"]), CachedInputTokens: intValue(nested(m, "prompt_tokens_details", "cached_tokens")), ReasoningTokens: intValue(nested(m, "completion_tokens_details", "reasoning_tokens"))}
}

func openAIResponsesUsage(v any) models.Usage {
	m, _ := v.(map[string]any)
	return models.Usage{InputTokens: intValue(m["input_tokens"]), OutputTokens: intValue(m["output_tokens"]), TotalTokens: intValue(m["total_tokens"]), CachedInputTokens: intValue(nested(m, "input_tokens_details", "cached_tokens")), ReasoningTokens: intValue(nested(m, "output_tokens_details", "reasoning_tokens"))}
}

func anthropicUsage(v any) models.Usage {
	m, _ := v.(map[string]any)
	input := intValue(m["input_tokens"])
	cacheRead := intValue(m["cache_read_input_tokens"])
	cacheWrite := intValue(m["cache_creation_input_tokens"])
	output := intValue(m["output_tokens"])
	return models.Usage{InputTokens: input + cacheRead + cacheWrite, OutputTokens: output, TotalTokens: input + cacheRead + cacheWrite + output, CachedInputTokens: cacheRead, CacheWriteTokens: cacheWrite}
}

func nested(v any, path ...string) any {
	cur := v
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	return cur
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
