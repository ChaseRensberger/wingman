package httpmodel

import (
	"context"
	"encoding/json"
	"fmt"
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
	GeminiGenerate    Protocol = "gemini_generate"
)

// Model is a small HTTP/SSE-backed implementation for the supported providers.
type Model struct {
	Info_           models.ModelInfo
	Protocol        Protocol
	BaseURL         string
	APIKey          string
	ForceStoreFalse bool
	Route           *Route
	Client          *http.Client
}

// Stream sends a streaming request and parses provider SSE into WingModels parts.
func (m *Model) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	route := m.route(req)
	body, err := m.body(req)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", m.Info_.Provider, err)
	}
	client := m.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := (streamTransport{client: client}).open(ctx, m.Info_.Provider, route, req.HTTP.Headers, bodyBytes)
	if err != nil {
		return nil, err
	}

	stream := models.NewEventStream[models.StreamPart, *models.Message](64)
	stream.BindContext(ctx)
	requestID := responseRequestID(resp.Header)
	go func() {
		defer resp.Body.Close()
		stream.Push(models.StreamStartPart{})
		if requestID != "" {
			stream.Push(models.ResponseMetadataPart{Meta: map[string]any{"request_id": requestID}})
		}
		msg, usage, reason, err := m.readSSE(ctx, resp.Body, stream)
		if msg != nil && !usage.Empty() {
			msg.Usage = &usage
		}
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

// responseRequestID checks x-request-id, request-id, openai-request-id, then
// x-goog-request-id, in priority order.
func responseRequestID(headers http.Header) string {
	for _, name := range []string{"x-request-id", "request-id", "openai-request-id", "x-goog-request-id"} {
		if requestID := strings.TrimSpace(headers.Get(name)); requestID != "" {
			return requestID
		}
	}
	return ""
}

// Prepare lowers a provider-neutral request into the provider JSON body without
// sending it.
func (m *Model) Prepare(ctx context.Context, req models.Request) (*models.PreparedRequest, error) {
	route := m.route(req)
	body, err := m.body(req)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"content-type": "application/json"}
	for k, v := range req.HTTP.Headers {
		headers[k] = v
	}
	for k, v := range route.Headers {
		headers[k] = v
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
		URL:     route.URL(),
		Headers: headers,
		Body:    body,
		Metadata: map[string]any{
			"route":    route.ID,
			"protocol": string(route.Protocol),
		},
	}, nil
}

// Generate drains Stream and returns the final assistant message.
func (m *Model) Generate(ctx context.Context, req models.Request) (*models.Message, error) {
	return models.Generate(ctx, m, req)
}

// LoweredOptions returns provider-calculated lowered request flags for the
// given request. It implements the optional LoweredOptionsProvider interface.
func (m *Model) LoweredOptions(ctx context.Context, req models.Request) models.LoweredOptions {
	var opts models.LoweredOptions
	switch m.Protocol {
	case OpenAIResponses:
		if req.Capabilities.Thinking {
			opts.ReasoningSummaryAuto = true
		}
	}
	return opts
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
	return m.route(models.Request{}).URL()
}

func (m *Model) route(req models.Request) Route {
	if m.Route != nil {
		route := *m.Route
		route.Endpoint.Query = mergeQuery(m.Protocol, route.Endpoint.Query, req.HTTP.Query)
		return route
	}
	return Route{
		ID:       string(m.Protocol),
		Protocol: m.Protocol,
		Endpoint: Endpoint{BaseURL: m.BaseURL, Query: defaultQuery(m.Protocol, req.HTTP.Query), ModelID: m.Info_.ID},
		Auth:     defaultAuth(m.Protocol, m.APIKey),
		Headers:  routeHeaders(m.Protocol),
	}
}

func defaultQuery(protocol Protocol, query map[string]string) map[string]string {
	return mergeQuery(protocol, nil, query)
}

func mergeQuery(protocol Protocol, configured, request map[string]string) map[string]string {
	out := make(map[string]string, len(configured)+len(request)+1)
	if protocol == GeminiGenerate {
		out["alt"] = "sse"
	}
	for k, v := range configured {
		out[k] = v
	}
	for k, v := range request {
		out[k] = v
	}
	return out
}

func (m *Model) auth(req *http.Request) {
	_ = m.route(models.Request{}).Apply(req)
}

func (m *Model) body(req models.Request) (map[string]any, error) {
	req.Messages = models.ExpandToolMessages(models.NormalizeMessages(req.Messages))
	switch m.Protocol {
	case OpenAIResponses:
		return m.openAIResponsesBody(req)
	case OpenAIChat:
		return m.openAIChatBody(req)
	case AnthropicMessages:
		return m.anthropicBody(req)
	case GeminiGenerate:
		return m.geminiBody(req), nil
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
			input = append(input, map[string]any{"role": "user", "content": openAIResponsesInputContent(msg.Content)})
		case models.RoleAssistant:
			input = append(input, openAIResponsesReasoningContent(msg.Content)...)
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
	if req.Capabilities.Thinking {
		body["reasoning"] = map[string]any{"summary": "auto"}
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	addTools(body, req.Tools, "responses")
	addOpenAIToolChoice(body, req.ToolChoice, "responses")
	addOpenAIResponseFormat(body, req.OutputSchema, req.ResponseFormat, "responses")
	addCommonOptions(body, req)
	body = m.applyRequestOptions(body, req)
	if m.ForceStoreFalse {
		body["store"] = false
	}
	return body, nil
}

func (m *Model) openAIChatBody(req models.Request) (map[string]any, error) {
	messages := make([]any, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, map[string]any{"role": "user", "content": openAIChatContent(msg.Content)})
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
	addOpenAIToolChoice(body, req.ToolChoice, "chat")
	addOpenAIResponseFormat(body, req.OutputSchema, req.ResponseFormat, "chat")
	addCommonOptions(body, req)
	return m.applyRequestOptions(body, req), nil
}

func (m *Model) anthropicBody(req models.Request) (map[string]any, error) {
	messages := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, map[string]any{"role": "user", "content": anthropicUserBlocks(msg.Content)})
		case models.RoleAssistant:
			content := anthropicAssistantBlocks(msg.Content)
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
		addAnthropicToolChoice(body, req.ToolChoice)
	}
	if req.Capabilities.Thinking {
		body["thinking"] = map[string]any{"type": "adaptive"}
	}
	addCommonOptions(body, req)
	return m.applyRequestOptions(body, req), nil
}

func (m *Model) geminiBody(req models.Request) map[string]any {
	contents := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			contents = append(contents, map[string]any{"role": "user", "parts": geminiUserParts(msg.Content)})
		case models.RoleAssistant:
			parts := geminiAssistantParts(msg.Content)
			for _, call := range toolCalls(msg.Content) {
				parts = append(parts, map[string]any{"functionCall": map[string]any{"name": call.Name, "args": call.Input}})
			}
			contents = append(contents, map[string]any{"role": "model", "parts": parts})
		case models.RoleTool:
			parts := make([]any, 0, len(msg.Content))
			for _, result := range toolResults(msg.Content) {
				response := map[string]any{"output": toolResultText(result)}
				if result.IsError {
					response["error"] = true
				}
				name := result.Name
				if name == "" {
					name = result.CallID
				}
				parts = append(parts, map[string]any{"functionResponse": map[string]any{"name": name, "response": response}})
			}
			contents = append(contents, map[string]any{"role": "user", "parts": parts})
		}
	}
	body := map[string]any{"contents": contents}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": req.System}}}
	}
	if len(req.Tools) > 0 && req.ToolChoice != models.ToolChoiceNone {
		declarations := make([]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			declarations = append(declarations, map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema})
		}
		body["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
		addGeminiToolChoice(body, req.ToolChoice)
	}
	if req.OutputSchema != nil || req.ResponseFormat.Type != "" {
		format := req.ResponseFormat
		if req.OutputSchema != nil {
			format = models.ResponseFormat{Type: "json_schema", Schema: req.OutputSchema.Schema}
		}
		if format.Type == "json_schema" || format.Type == "json" {
			generationConfig := generationConfig(body)
			generationConfig["responseMimeType"] = "application/json"
			if format.Schema != nil {
				generationConfig["responseSchema"] = format.Schema
			}
		}
	}
	addGeminiOptions(body, req)
	return m.applyRequestOptions(body, req)
}

func (m *Model) applyRequestOptions(body map[string]any, req models.Request) map[string]any {
	body = overlay(body, req.ProviderOptions[m.Info_.Provider])
	return overlay(body, req.HTTP.Body)
}

type parseState struct {
	provider string
	api      models.API
	model    string
	text     strings.Builder
	textID   string
	reason   strings.Builder
	reasonID string
	sig      string
	tools    []models.ToolCallPart
	usage    models.Usage
	finish   models.FinishReason
	toolBuf  map[string]*toolAccum
}

type toolAccum struct {
	id      string
	name    string
	args    strings.Builder
	started bool
}

func (s *parseState) message() *models.Message {
	content := models.Content{}
	if text := s.text.String(); text != "" {
		content = append(content, models.TextPart{Text: text})
	}
	if reasoning := s.reason.String(); reasoning != "" {
		part := models.ReasoningPart{Reasoning: reasoning, Encrypted: s.sig}
		if s.api == models.APIOpenAIResponses && (s.reasonID != "" || s.sig != "") {
			part.ProviderMetadata = models.Meta{"openai": map[string]any{"item_id": s.reasonID, "reasoning_encrypted_content": s.sig}}
		}
		content = append(content, part)
	}
	for _, call := range s.tools {
		content = append(content, call)
	}
	return &models.Message{Role: models.RoleAssistant, Content: content, FinishReason: s.finish, Origin: &models.MessageOrigin{Provider: s.provider, API: s.api, ModelID: s.model}}
}

func parseOpenAIResponses(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	typeName, _ := event["type"].(string)
	switch typeName {
	case "response.output_text.delta":
		if delta, _ := event["delta"].(string); delta != "" {
			pushText(state, stream, "text-0", delta)
		}
	case "response.reasoning_text.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta":
		if delta, _ := event["delta"].(string); delta != "" {
			id := stringValue(event["item_id"])
			if id == "" {
				id = "reasoning-0"
			}
			pushReasoning(state, stream, id, delta)
		}
	case "response.output_item.done":
		item, _ := event["item"].(map[string]any)
		if itemType, _ := item["type"].(string); itemType == "reasoning" {
			if id := stringValue(item["id"]); id != "" {
				state.reasonID = id
			}
			if encrypted := stringValue(item["encrypted_content"]); encrypted != "" {
				state.sig = encrypted
			}
		}
		if itemType, _ := item["type"].(string); itemType == "function_call" {
			itemID := stringValue(item["id"])
			arguments := stringValue(item["arguments"])
			if arguments == "" && state.toolBuf != nil {
				if acc := state.toolBuf[itemID]; acc != nil {
					arguments = acc.args.String()
				}
			}
			input, err := decodeArgs(arguments)
			if err != nil {
				return decodingError(state.provider, "invalid tool arguments", err)
			}
			call := models.ToolCallPart{CallID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Input: input}
			pushToolRaw(state, stream, call, arguments)
			delete(state.toolBuf, itemID)
		}
	case "response.function_call_arguments.delta":
		itemID := stringValue(event["item_id"])
		if itemID == "" {
			return nil
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
		state.finish = finishReason(stringValue(nested(event, "response", "incomplete_details", "reason")), len(state.tools) > 0)
		state.usage = openAIResponsesUsage(nested(event, "response", "usage"))
	}
	return nil
}

func parseOpenAIChat(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	choices, _ := event["choices"].([]any)
	var choiceUsage models.Usage
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		choiceUsage = openAIChatUsage(choice["usage"])
		delta, _ := choice["delta"].(map[string]any)
		if text, _ := delta["content"].(string); text != "" {
			pushText(state, stream, "text-0", text)
		}
		if reasoning := stringValue(delta["reasoning_content"]); reasoning != "" {
			pushReasoning(state, stream, "reasoning-0", reasoning)
		}
		if calls, _ := delta["tool_calls"].([]any); len(calls) > 0 {
			if state.toolBuf == nil {
				state.toolBuf = map[string]*toolAccum{}
			}
			for _, raw := range calls {
				call, _ := raw.(map[string]any)
				index, err := toolCallIndex(call["index"])
				if err != nil {
					return decodingError(state.provider, "invalid OpenAI Chat tool call index", err)
				}
				idx := fmt.Sprint(index)
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
				fragment := stringValue(fn["arguments"])
				if fragment != "" {
					if !acc.started {
						stream.Push(models.ToolInputStartPart{ID: acc.id, ToolName: acc.name})
						acc.started = true
					}
					acc.args.WriteString(fragment)
					stream.Push(models.ToolInputDeltaPart{ID: acc.id, Delta: fragment})
				}
			}
		}
		if reason := stringValue(choice["finish_reason"]); reason != "" {
			for index := 0; len(state.toolBuf) > 0; index++ {
				acc, ok := state.toolBuf[fmt.Sprint(index)]
				if !ok {
					return decodingError(state.provider, "non-contiguous OpenAI Chat tool call indices", nil)
				}
				input, err := decodeArgs(acc.args.String())
				if err != nil {
					return decodingError(state.provider, "invalid tool arguments", err)
				}
				call := models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: input}
				if acc.started {
					completeTool(state, stream, call)
				} else {
					pushToolRaw(state, stream, call, acc.args.String())
				}
				delete(state.toolBuf, fmt.Sprint(index))
			}
			state.finish = finishReason(reason, len(state.tools) > 0)
		}
	}
	if usage := openAIChatUsage(event["usage"]); !usage.Empty() {
		state.usage = usage
	} else if !choiceUsage.Empty() {
		state.usage = choiceUsage
	}
	return nil
}

func parseAnthropic(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	typeName := stringValue(event["type"])
	switch typeName {
	case "content_block_start":
		block, _ := event["content_block"].(map[string]any)
		if stringValue(block["type"]) != "tool_use" {
			return nil
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
			return nil
		}
		if thinking := stringValue(delta["thinking"]); thinking != "" {
			pushReasoning(state, stream, fmt.Sprintf("reasoning-%d", intValue(event["index"])), thinking)
			return nil
		}
		if stringValue(delta["type"]) == "input_json_delta" {
			idx := fmt.Sprint(intValue(event["index"]))
			if state.toolBuf == nil || state.toolBuf[idx] == nil {
				return nil
			}
			fragment := stringValue(delta["partial_json"])
			state.toolBuf[idx].args.WriteString(fragment)
			stream.Push(models.ToolInputDeltaPart{ID: state.toolBuf[idx].id, Delta: fragment})
		}
	case "content_block_stop":
		idx := fmt.Sprint(intValue(event["index"]))
		if state.toolBuf == nil || state.toolBuf[idx] == nil {
			return nil
		}
		acc := state.toolBuf[idx]
		stream.Push(models.ToolInputEndPart{ID: acc.id})
		input, err := decodeArgs(acc.args.String())
		if err != nil {
			return decodingError(state.provider, "invalid tool arguments", err)
		}
		call := models.ToolCallPart{CallID: acc.id, Name: acc.name, Input: input}
		state.tools = append(state.tools, call)
		stream.Push(models.ToolCallPart_{ID: call.CallID, ToolName: call.Name, Input: call.Input})
		delete(state.toolBuf, idx)
	case "message_delta":
		delta, _ := event["delta"].(map[string]any)
		state.finish = finishReason(stringValue(delta["stop_reason"]), len(state.tools) > 0)
		state.usage = anthropicUsage(event["usage"])
	}
	return nil
}

func parseGemini(event map[string]any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	if usage := geminiUsage(event["usageMetadata"]); !usage.Empty() {
		state.usage = usage
	}
	choices, _ := event["candidates"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	content, _ := choice["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	for _, rawPart := range parts {
		part, _ := rawPart.(map[string]any)
		if sig := stringValue(part["thoughtSignature"]); sig != "" {
			state.sig = sig
		}
		if text := stringValue(part["text"]); text != "" {
			if thought, _ := part["thought"].(bool); thought {
				pushReasoning(state, stream, "reasoning-0", text)
				continue
			}
			pushText(state, stream, "text-0", text)
		}
		if rawCall, _ := part["functionCall"].(map[string]any); rawCall != nil {
			args, ok := rawCall["args"].(map[string]any)
			if !ok {
				return decodingError(state.provider, "invalid Gemini function arguments", nil)
			}
			call := models.ToolCallPart{
				CallID: fmt.Sprintf("call_%d", len(state.tools)+1),
				Name:   stringValue(rawCall["name"]),
				Input:  args,
			}
			pushTool(state, stream, call)
		}
	}
	if reason := stringValue(choice["finishReason"]); reason != "" {
		state.finish = finishReason(reason, len(state.tools) > 0)
	}
	return nil
}

func pushText(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], id, delta string) {
	if state.text.Len() == 0 {
		state.textID = id
		stream.Push(models.TextStartPart{ID: id})
	}
	state.text.WriteString(delta)
	stream.Push(models.TextDeltaPart{ID: id, Delta: delta})
}

func pushReasoning(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], id, delta string) {
	if state.reason.Len() == 0 {
		state.reasonID = id
		stream.Push(models.ReasoningStartPart{ID: id})
	}
	state.reason.WriteString(delta)
	stream.Push(models.ReasoningDeltaPart{ID: id, Delta: delta})
}

func closeOpenParts(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) {
	if state.reason.Len() > 0 && state.reasonID != "" {
		part := models.ReasoningEndPart{ID: state.reasonID}
		if state.sig != "" {
			part.ProviderMetadata = models.Meta{"google": map[string]any{"thought_signature": state.sig}}
		}
		stream.Push(part)
	}
	if state.text.Len() > 0 && state.textID != "" {
		stream.Push(models.TextEndPart{ID: state.textID})
	}
}

func pushTool(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], call models.ToolCallPart) {
	pushToolRaw(state, stream, call, encodeJSON(call.Input))
}

func pushToolRaw(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], call models.ToolCallPart, raw string) {
	stream.Push(models.ToolInputStartPart{ID: call.CallID, ToolName: call.Name})
	stream.Push(models.ToolInputDeltaPart{ID: call.CallID, Delta: raw})
	completeTool(state, stream, call)
}

func completeTool(state *parseState, stream *models.EventStream[models.StreamPart, *models.Message], call models.ToolCallPart) {
	state.tools = append(state.tools, call)
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

func addOpenAIToolChoice(body map[string]any, choice models.ToolChoice, mode string) {
	switch choice {
	case "", models.ToolChoiceAuto:
		return
	case models.ToolChoiceNone:
		body["tool_choice"] = "none"
	case models.ToolChoiceRequired:
		body["tool_choice"] = "required"
	default:
		if mode == "chat" {
			body["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": string(choice)}}
			return
		}
		body["tool_choice"] = map[string]any{"type": "function", "name": string(choice)}
	}
}

func addAnthropicToolChoice(body map[string]any, choice models.ToolChoice) {
	switch choice {
	case "", models.ToolChoiceAuto:
		return
	case models.ToolChoiceNone:
		delete(body, "tools")
	case models.ToolChoiceRequired:
		body["tool_choice"] = map[string]any{"type": "any"}
	default:
		body["tool_choice"] = map[string]any{"type": "tool", "name": string(choice)}
	}
}

func addGeminiToolChoice(body map[string]any, choice models.ToolChoice) {
	var cfg map[string]any
	switch choice {
	case "", models.ToolChoiceAuto:
		return
	case models.ToolChoiceNone:
		delete(body, "tools")
		return
	case models.ToolChoiceRequired:
		cfg = map[string]any{"mode": "ANY"}
	default:
		cfg = map[string]any{"mode": "ANY", "allowedFunctionNames": []string{string(choice)}}
	}
	body["toolConfig"] = map[string]any{"functionCallingConfig": cfg}
}

func addOpenAIResponseFormat(body map[string]any, schema *models.OutputSchema, format models.ResponseFormat, mode string) {
	if schema != nil {
		format = models.ResponseFormat{Type: "json_schema", Name: schema.Name, Schema: schema.Schema, Strict: schema.Strict}
	}
	if format.Type == "" {
		return
	}
	if mode == "chat" {
		if format.Type == "json_schema" {
			body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": defaultName(format.Name), "schema": format.Schema, "strict": format.Strict}}
			return
		}
		body["response_format"] = map[string]any{"type": format.Type}
		return
	}
	if format.Type == "json_schema" {
		body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": defaultName(format.Name), "schema": format.Schema, "strict": format.Strict}}
		return
	}
	body["text"] = map[string]any{"format": map[string]any{"type": format.Type}}
}

func defaultName(name string) string {
	if name != "" {
		return name
	}
	return "output"
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

func addGeminiOptions(body map[string]any, req models.Request) {
	config := generationConfig(body)
	if req.Generation.MaxTokens != 0 {
		config["maxOutputTokens"] = req.Generation.MaxTokens
	} else if req.MaxOutputTokens != 0 {
		config["maxOutputTokens"] = req.MaxOutputTokens
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
	if len(config) == 0 {
		delete(body, "generationConfig")
	}
}

func generationConfig(body map[string]any) map[string]any {
	if raw, ok := body["generationConfig"].(map[string]any); ok {
		return raw
	}
	config := map[string]any{}
	body["generationConfig"] = config
	return config
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

func openAIResponsesReasoningContent(content models.Content) []any {
	var out []any
	for _, part := range content {
		reasoning, ok := part.(models.ReasoningPart)
		if !ok || reasoning.Encrypted == "" {
			continue
		}
		openAI, ok := reasoning.ProviderMetadata["openai"].(map[string]any)
		if !ok || stringValue(openAI["reasoning_encrypted_content"]) != reasoning.Encrypted {
			continue
		}
		itemID := stringValue(openAI["item_id"])
		if itemID == "" {
			continue
		}
		out = append(out, map[string]any{"type": "reasoning", "id": itemID, "summary": []any{}, "encrypted_content": reasoning.Encrypted})
	}
	return out
}

func openAIResponsesInputContent(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"type": "input_text", "text": p.Text})
		case models.ImagePart:
			if url := imageURL(p); url != "" {
				out = append(out, map[string]any{"type": "input_image", "image_url": url})
			}
		}
	}
	return out
}

func openAIChatContent(content models.Content) any {
	if !hasImage(content) {
		return joinText(content)
	}
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case models.ImagePart:
			if url := imageURL(p); url != "" {
				out = append(out, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
			}
		}
	}
	return out
}

func anthropicUserBlocks(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case models.ImagePart:
			if p.Base64 != "" {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "image/png"
				}
				out = append(out, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mediaType, "data": p.Base64}})
			}
		}
	}
	return out
}

func anthropicAssistantBlocks(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case models.ReasoningPart:
			block := map[string]any{"type": "thinking", "thinking": p.Reasoning}
			if p.Encrypted != "" {
				block["signature"] = p.Encrypted
			}
			out = append(out, block)
		}
	}
	return out
}

func geminiUserParts(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"text": p.Text})
		case models.ImagePart:
			if p.Base64 != "" {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "image/png"
				}
				out = append(out, map[string]any{"inlineData": map[string]any{"mimeType": mediaType, "data": p.Base64}})
			}
		}
	}
	return out
}

func geminiAssistantParts(content models.Content) []any {
	out := []any{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, map[string]any{"text": p.Text})
		case models.ReasoningPart:
			part := map[string]any{"text": p.Reasoning, "thought": true}
			if p.Encrypted != "" {
				part["thoughtSignature"] = p.Encrypted
			}
			out = append(out, part)
		}
	}
	return out
}

func imageURL(p models.ImagePart) string {
	if p.URL != "" {
		return p.URL
	}
	if p.Base64 == "" {
		return ""
	}
	mediaType := p.MediaType
	if mediaType == "" {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + p.Base64
}

func hasImage(content models.Content) bool {
	for _, part := range content {
		if _, ok := part.(models.ImagePart); ok {
			return true
		}
	}
	return false
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

func decodeArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("tool arguments must be a JSON object")
	}
	return out, nil
}

func toolCallIndex(v any) (int, error) {
	n, ok := v.(float64)
	if !ok || n < 0 || n != float64(int(n)) {
		return 0, fmt.Errorf("index must be a non-negative integer")
	}
	return int(n), nil
}

func finishReason(raw string, hasTools bool) models.FinishReason {
	raw = strings.ToLower(raw)
	if hasTools || raw == "tool_calls" || raw == "function_call" || raw == "tool_use" {
		return models.FinishReasonToolCalls
	}
	if raw == "length" || raw == "max_tokens" || raw == "max_output_tokens" {
		return models.FinishReasonMaxTokens
	}
	if raw == "image_safety" || raw == "recitation" || raw == "safety" || raw == "blocklist" || raw == "prohibited_content" || raw == "spii" {
		return models.FinishReasonBlocked
	}
	if raw == "malformed_function_call" {
		return models.FinishReasonError
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

func geminiUsage(v any) models.Usage {
	m, _ := v.(map[string]any)
	input := intValue(m["promptTokenCount"])
	output := intValue(m["candidatesTokenCount"])
	total := intValue(m["totalTokenCount"])
	if total == 0 {
		total = input + output
	}
	return models.Usage{InputTokens: input, OutputTokens: output, TotalTokens: total, CachedInputTokens: intValue(m["cachedContentTokenCount"]), ReasoningTokens: intValue(m["thoughtsTokenCount"])}
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
