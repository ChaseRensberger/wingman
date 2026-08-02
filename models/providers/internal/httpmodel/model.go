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
		return m.geminiBody(req)
	default:
		return nil, fmt.Errorf("unsupported protocol %q", m.Protocol)
	}
}

func (m *Model) openAIResponsesBody(req models.Request) (map[string]any, error) {
	input := make([]any, 0, len(req.Messages)+1)
	if req.System != "" {
		input = append(input, openAIResponsesInputItem{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			input = append(input, openAIResponsesInputItem{Role: "user", Content: openAIResponsesInputContent(msg.Content)})
		case models.RoleAssistant:
			input = append(input, openAIResponsesReasoningContent(msg.Content)...)
			texts := openAIResponsesTextContent(msg.Content, "output_text")
			if len(texts) > 0 {
				input = append(input, openAIResponsesInputItem{Role: "assistant", Content: texts})
			}
			for _, call := range toolCalls(msg.Content) {
				input = append(input, openAIResponsesInputItem{Type: "function_call", CallID: call.CallID, Name: call.Name, Arguments: encodeJSON(call.Input)})
			}
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				input = append(input, openAIResponsesInputItem{Type: "function_call_output", CallID: result.CallID, Output: toolResultText(result)})
			}
		}
	}
	body, err := jsonObject(openAIResponsesRequest{Model: m.Info_.ID, Input: input, Stream: true})
	if err != nil {
		return nil, err
	}
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

type openAIResponsesRequest struct {
	Model  string `json:"model"`
	Input  []any  `json:"input"`
	Stream bool   `json:"stream"`
}

type openAIResponsesInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

func (m *Model) openAIChatBody(req models.Request) (map[string]any, error) {
	messages := make([]openAIChatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openAIChatMessage{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, openAIChatMessage{Role: "user", Content: openAIChatContent(msg.Content)})
		case models.RoleAssistant:
			message := openAIChatMessage{Role: "assistant", Content: joinText(msg.Content)}
			if calls := toolCalls(msg.Content); len(calls) > 0 {
				message.ToolCalls = make([]openAIChatToolCall, 0, len(calls))
				for _, call := range calls {
					message.ToolCalls = append(message.ToolCalls, openAIChatToolCall{ID: call.CallID, Type: "function", Function: openAIChatFunction{Name: call.Name, Arguments: encodeJSON(call.Input)}})
				}
			}
			messages = append(messages, message)
		case models.RoleTool:
			for _, result := range toolResults(msg.Content) {
				messages = append(messages, openAIChatMessage{Role: "tool", ToolCallID: result.CallID, Content: toolResultText(result)})
			}
		}
	}
	body, err := jsonObject(openAIChatRequest{Model: m.Info_.ID, Messages: messages, Stream: true, StreamOptions: openAIChatStreamOptions{IncludeUsage: true}})
	if err != nil {
		return nil, err
	}
	addTools(body, req.Tools, "chat")
	addOpenAIToolChoice(body, req.ToolChoice, "chat")
	addOpenAIResponseFormat(body, req.OutputSchema, req.ResponseFormat, "chat")
	addCommonOptions(body, req)
	return m.applyRequestOptions(body, req), nil
}

type openAIChatRequest struct {
	Model         string                  `json:"model"`
	Messages      []openAIChatMessage     `json:"messages"`
	Stream        bool                    `json:"stream"`
	StreamOptions openAIChatStreamOptions `json:"stream_options"`
}

type openAIChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}
type openAIChatMessage struct {
	Role       string               `json:"role"`
	Content    any                  `json:"content"`
	ToolCalls  []openAIChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}
type openAIChatToolCall struct {
	Index    *int               `json:"index,omitempty"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIChatFunction `json:"function"`
}
type openAIChatFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
type openAIChatUsageEvent struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}
type openAIChatChoice struct {
	Delta        openAIChatDelta       `json:"delta"`
	FinishReason string                `json:"finish_reason"`
	Usage        *openAIChatUsageEvent `json:"usage"`
}
type openAIChatDelta struct {
	Content          string               `json:"content"`
	ReasoningContent string               `json:"reasoning_content"`
	ToolCalls        []openAIChatToolCall `json:"tool_calls"`
}
type openAIChatEvent struct {
	Choices []openAIChatChoice    `json:"choices"`
	Usage   *openAIChatUsageEvent `json:"usage"`
}

func jsonObject(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	return object, nil
}

func (m *Model) anthropicBody(req models.Request) (map[string]any, error) {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			messages = append(messages, anthropicMessage{Role: "user", Content: anthropicUserBlocks(msg.Content)})
		case models.RoleAssistant:
			content := anthropicAssistantBlocks(msg.Content)
			for _, call := range toolCalls(msg.Content) {
				content = append(content, anthropicContentBlock{Type: "tool_use", ID: call.CallID, Name: call.Name, Input: call.Input})
			}
			messages = append(messages, anthropicMessage{Role: "assistant", Content: content})
		case models.RoleTool:
			content := make([]anthropicContentBlock, 0, len(msg.Content))
			for _, result := range toolResults(msg.Content) {
				content = append(content, anthropicContentBlock{Type: "tool_result", ToolUseID: result.CallID, Content: anthropicToolResultContent(result), IsError: result.IsError})
			}
			messages = append(messages, anthropicMessage{Role: "user", Content: content})
		}
	}
	body, err := jsonObject(anthropicRequest{Model: m.Info_.ID, Messages: messages, Stream: true, MaxTokens: maxOutput(req, m.Info_.MaxOutput)})
	if err != nil {
		return nil, err
	}
	if req.System != "" {
		body["system"] = []anthropicContentBlock{{Type: "text", Text: req.System}}
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

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	Thinking  string           `json:"thinking,omitempty"`
	Signature string           `json:"signature,omitempty"`
	Source    *anthropicSource `json:"source,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     map[string]any   `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   any              `json:"content,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

func (m *Model) geminiBody(req models.Request) (map[string]any, error) {
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case models.RoleUser:
			contents = append(contents, geminiContent{Role: "user", Parts: geminiUserParts(msg.Content)})
		case models.RoleAssistant:
			parts := geminiAssistantParts(msg.Content)
			for _, call := range toolCalls(msg.Content) {
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{Name: call.Name, Args: call.Input}})
			}
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		case models.RoleTool:
			parts := make([]geminiPart, 0, len(msg.Content))
			for _, result := range toolResults(msg.Content) {
				name := result.Name
				if name == "" {
					name = result.CallID
				}
				parts = append(parts, geminiPart{FunctionResponse: &geminiFunctionResponse{Name: name, Response: geminiToolResultResponse(result)}})
			}
			contents = append(contents, geminiContent{Role: "user", Parts: parts})
		}
	}
	body, err := jsonObject(geminiRequest{Contents: contents})
	if err != nil {
		return nil, err
	}
	if req.System != "" {
		body["systemInstruction"] = geminiContent{Parts: []geminiPart{{Text: req.System}}}
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
	return m.applyRequestOptions(body, req), nil
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}
type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}
type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}
type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
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

type openAIResponsesEvent struct {
	Type     string                    `json:"type"`
	Delta    string                    `json:"delta"`
	ItemID   string                    `json:"item_id"`
	Item     openAIResponsesOutputItem `json:"item"`
	Response openAIResponsesResponse   `json:"response"`
}

type openAIResponsesOutputItem struct {
	Type             string `json:"type"`
	ID               string `json:"id"`
	CallID           string `json:"call_id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	EncryptedContent string `json:"encrypted_content"`
}

type openAIResponsesResponse struct {
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage openAIResponsesUsageEvent `json:"usage"`
}

type openAIResponsesUsageEvent struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func parseOpenAIResponses(event openAIResponsesEvent, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	switch event.Type {
	case "response.output_text.delta":
		if delta := event.Delta; delta != "" {
			pushText(state, stream, "text-0", delta)
		}
	case "response.reasoning_text.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta":
		if delta := event.Delta; delta != "" {
			id := event.ItemID
			if id == "" {
				id = "reasoning-0"
			}
			pushReasoning(state, stream, id, delta)
		}
	case "response.output_item.done":
		item := event.Item
		if item.Type == "reasoning" {
			if id := item.ID; id != "" {
				state.reasonID = id
			}
			if encrypted := item.EncryptedContent; encrypted != "" {
				state.sig = encrypted
			}
		}
		if item.Type == "function_call" {
			itemID := item.ID
			arguments := item.Arguments
			if arguments == "" && state.toolBuf != nil {
				if acc := state.toolBuf[itemID]; acc != nil {
					arguments = acc.args.String()
				}
			}
			input, err := decodeArgs(arguments)
			if err != nil {
				return decodingError(state.provider, "invalid tool arguments", err)
			}
			call := models.ToolCallPart{CallID: item.CallID, Name: item.Name, Input: input}
			pushToolRaw(state, stream, call, arguments)
			delete(state.toolBuf, itemID)
		}
	case "response.function_call_arguments.delta":
		itemID := event.ItemID
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
		acc.args.WriteString(event.Delta)
	case "response.completed", "response.incomplete":
		state.finish = finishReason(event.Response.IncompleteDetails.Reason, len(state.tools) > 0)
		state.usage = openAIResponsesUsage(event.Response.Usage)
	}
	return nil
}

func parseOpenAIChat(raw any, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	event, ok := raw.(openAIChatEvent)
	if !ok {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return decodingError(state.provider, "invalid OpenAI Chat event", err)
		}
		if err := json.Unmarshal(encoded, &event); err != nil {
			return decodingError(state.provider, "invalid OpenAI Chat event", err)
		}
	}
	choices := event.Choices
	var choiceUsage models.Usage
	if len(choices) > 0 {
		choice := choices[0]
		choiceUsage = openAIChatUsage(choice.Usage)
		delta := choice.Delta
		if text := delta.Content; text != "" {
			pushText(state, stream, "text-0", text)
		}
		if reasoning := delta.ReasoningContent; reasoning != "" {
			pushReasoning(state, stream, "reasoning-0", reasoning)
		}
		if calls := delta.ToolCalls; len(calls) > 0 {
			if state.toolBuf == nil {
				state.toolBuf = map[string]*toolAccum{}
			}
			for _, call := range calls {
				if call.Index == nil {
					return decodingError(state.provider, "invalid OpenAI Chat tool call index", nil)
				}
				index := *call.Index
				idx := fmt.Sprint(index)
				acc := state.toolBuf[idx]
				if acc == nil {
					acc = &toolAccum{}
					state.toolBuf[idx] = acc
				}
				if id := call.ID; id != "" {
					acc.id = id
				}
				if name := call.Function.Name; name != "" {
					acc.name = name
				}
				fragment := call.Function.Arguments
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
		if reason := choice.FinishReason; reason != "" {
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
	if usage := openAIChatUsage(event.Usage); !usage.Empty() {
		state.usage = usage
	} else if !choiceUsage.Empty() {
		state.usage = choiceUsage
	}
	return nil
}

type anthropicEvent struct {
	Type         string                   `json:"type"`
	Index        int                      `json:"index"`
	ContentBlock anthropicSSEContentBlock `json:"content_block"`
	Delta        anthropicSSEDelta        `json:"delta"`
	Usage        anthropicUsageEvent      `json:"usage"`
}
type anthropicSSEContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}
type anthropicSSEDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}
type anthropicUsageEvent struct {
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

func parseAnthropic(event anthropicEvent, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	switch event.Type {
	case "content_block_start":
		block := event.ContentBlock
		if block.Type != "tool_use" {
			return nil
		}
		if state.toolBuf == nil {
			state.toolBuf = map[string]*toolAccum{}
		}
		idx := fmt.Sprint(event.Index)
		state.toolBuf[idx] = &toolAccum{id: block.ID, name: block.Name}
		stream.Push(models.ToolInputStartPart{ID: block.ID, ToolName: block.Name})
	case "content_block_delta":
		delta := event.Delta
		if text := delta.Text; text != "" {
			pushText(state, stream, fmt.Sprintf("text-%d", event.Index), text)
			return nil
		}
		if thinking := delta.Thinking; thinking != "" {
			pushReasoning(state, stream, fmt.Sprintf("reasoning-%d", event.Index), thinking)
			return nil
		}
		if delta.Type == "input_json_delta" {
			idx := fmt.Sprint(event.Index)
			if state.toolBuf == nil || state.toolBuf[idx] == nil {
				return nil
			}
			fragment := delta.PartialJSON
			state.toolBuf[idx].args.WriteString(fragment)
			stream.Push(models.ToolInputDeltaPart{ID: state.toolBuf[idx].id, Delta: fragment})
		}
	case "content_block_stop":
		idx := fmt.Sprint(event.Index)
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
		state.finish = finishReason(event.Delta.StopReason, len(state.tools) > 0)
		state.usage = anthropicUsage(event.Usage)
	}
	return nil
}

type geminiEvent struct {
	UsageMetadata geminiUsageEvent  `json:"usageMetadata"`
	Candidates    []geminiCandidate `json:"candidates"`
}
type geminiUsageEvent struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
}
type geminiCandidate struct {
	Content      geminiSSEContent `json:"content"`
	FinishReason string           `json:"finishReason"`
}
type geminiSSEContent struct {
	Parts []geminiSSEPart `json:"parts"`
}
type geminiSSEPart struct {
	Text             string              `json:"text"`
	Thought          bool                `json:"thought"`
	ThoughtSignature string              `json:"thoughtSignature"`
	FunctionCall     *geminiFunctionCall `json:"functionCall"`
}

func parseGemini(event geminiEvent, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	if usage := geminiUsage(event.UsageMetadata); !usage.Empty() {
		state.usage = usage
	}
	choices := event.Candidates
	if len(choices) == 0 {
		return nil
	}
	choice := choices[0]
	for _, part := range choice.Content.Parts {
		if sig := part.ThoughtSignature; sig != "" {
			state.sig = sig
		}
		if text := part.Text; text != "" {
			if part.Thought {
				pushReasoning(state, stream, "reasoning-0", text)
				continue
			}
			pushText(state, stream, "text-0", text)
		}
		if rawCall := part.FunctionCall; rawCall != nil {
			if rawCall.Args == nil {
				return decodingError(state.provider, "invalid Gemini function arguments", nil)
			}
			call := models.ToolCallPart{
				CallID: fmt.Sprintf("call_%d", len(state.tools)+1),
				Name:   rawCall.Name,
				Input:  rawCall.Args,
			}
			pushTool(state, stream, call)
		}
	}
	if reason := choice.FinishReason; reason != "" {
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

func openAIResponsesTextContent(content models.Content, typ string) []openAIResponsesContent {
	out := []openAIResponsesContent{}
	for _, part := range content {
		if t, ok := part.(models.TextPart); ok {
			out = append(out, openAIResponsesContent{Type: typ, Text: t.Text})
		}
	}
	return out
}

type openAIResponsesContent struct {
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	ImageURL         string `json:"image_url,omitempty"`
	ID               string `json:"id,omitempty"`
	Summary          []any  `json:"summary,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
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
		out = append(out, openAIResponsesContent{Type: "reasoning", ID: itemID, Summary: []any{}, EncryptedContent: reasoning.Encrypted})
	}
	return out
}

func openAIResponsesInputContent(content models.Content) []openAIResponsesContent {
	out := []openAIResponsesContent{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, openAIResponsesContent{Type: "input_text", Text: p.Text})
		case models.ImagePart:
			if url := imageURL(p); url != "" {
				out = append(out, openAIResponsesContent{Type: "input_image", ImageURL: url})
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

func anthropicUserBlocks(content models.Content) []anthropicContentBlock {
	out := []anthropicContentBlock{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, anthropicContentBlock{Type: "text", Text: p.Text})
		case models.ImagePart:
			if p.Base64 != "" {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "image/png"
				}
				out = append(out, anthropicContentBlock{Type: "image", Source: &anthropicSource{Type: "base64", MediaType: mediaType, Data: p.Base64}})
			}
		}
	}
	return out
}

func anthropicAssistantBlocks(content models.Content) []anthropicContentBlock {
	out := []anthropicContentBlock{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, anthropicContentBlock{Type: "text", Text: p.Text})
		case models.ReasoningPart:
			out = append(out, anthropicContentBlock{Type: "thinking", Thinking: p.Reasoning, Signature: p.Encrypted})
		}
	}
	return out
}

func geminiUserParts(content models.Content) []geminiPart {
	out := []geminiPart{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, geminiPart{Text: p.Text})
		case models.ImagePart:
			if p.Base64 != "" {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "image/png"
				}
				out = append(out, geminiPart{InlineData: &geminiInlineData{MimeType: mediaType, Data: p.Base64}})
			}
		}
	}
	return out
}

func geminiAssistantParts(content models.Content) []geminiPart {
	out := []geminiPart{}
	for _, part := range content {
		switch p := part.(type) {
		case models.TextPart:
			out = append(out, geminiPart{Text: p.Text})
		case models.ReasoningPart:
			out = append(out, geminiPart{Text: p.Reasoning, Thought: true, ThoughtSignature: p.Encrypted})
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

func anthropicToolResultContent(part models.ToolResultPart) any {
	blocks := make([]anthropicContentBlock, 0, len(part.Output)+1)
	for _, output := range part.Output {
		switch p := output.(type) {
		case models.TextPart:
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
		case models.ImagePart:
			if p.Base64 == "" {
				continue
			}
			mediaType := p.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			blocks = append(blocks, anthropicContentBlock{Type: "image", Source: &anthropicSource{Type: "base64", MediaType: mediaType, Data: p.Base64}})
		}
	}
	if part.Structured != nil {
		blocks = append(blocks, anthropicContentBlock{Type: "text", Text: encodeJSON(part.Structured)})
	}
	if len(blocks) == 1 && blocks[0].Type == "text" {
		return blocks[0].Text
	}
	return blocks
}

func geminiToolResultResponse(part models.ToolResultPart) map[string]any {
	response := map[string]any{"output": toolResultText(part)}
	if part.Structured != nil {
		response["structured"] = part.Structured
	}
	if part.IsError {
		response["error"] = true
	}
	return response
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

func openAIChatUsage(v *openAIChatUsageEvent) models.Usage {
	if v == nil {
		return models.Usage{}
	}
	return models.Usage{InputTokens: v.PromptTokens, OutputTokens: v.CompletionTokens, TotalTokens: v.TotalTokens, CachedInputTokens: v.PromptTokensDetails.CachedTokens, ReasoningTokens: v.CompletionTokensDetails.ReasoningTokens}
}

func openAIResponsesUsage(v openAIResponsesUsageEvent) models.Usage {
	return models.Usage{InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, TotalTokens: v.TotalTokens, CachedInputTokens: v.InputTokensDetails.CachedTokens, ReasoningTokens: v.OutputTokensDetails.ReasoningTokens}
}

func anthropicUsage(v anthropicUsageEvent) models.Usage {
	input := v.InputTokens
	cacheRead := v.CacheReadInputTokens
	cacheWrite := v.CacheCreationInputTokens
	output := v.OutputTokens
	return models.Usage{InputTokens: input + cacheRead + cacheWrite, OutputTokens: output, TotalTokens: input + cacheRead + cacheWrite + output, CachedInputTokens: cacheRead, CacheWriteTokens: cacheWrite}
}

func geminiUsage(v geminiUsageEvent) models.Usage {
	input := v.PromptTokenCount
	output := v.CandidatesTokenCount
	total := v.TotalTokenCount
	if total == 0 {
		total = input + output
	}
	return models.Usage{InputTokens: input, OutputTokens: output, TotalTokens: total, CachedInputTokens: v.CachedContentTokenCount, ReasoningTokens: v.ThoughtsTokenCount}
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
