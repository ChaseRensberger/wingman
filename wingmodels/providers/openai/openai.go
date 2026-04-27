// Package openai implements wingmodels.Model for OpenAI's Responses API.
//
// Wire reference: https://platform.openai.com/docs/api-reference/responses
//
// Stream mapping (Responses SSE -> wingmodels.StreamPart):
//
//	response.created                          -> StreamStartPart{} + ResponseMetadataPart{id}
//	response.output_item.added (reasoning)    -> ReasoningStartPart{id=item_index}
//	response.reasoning_summary_text.delta     -> ReasoningDeltaPart{id, delta}
//	response.output_item.done (reasoning)     -> ReasoningEndPart{id}
//	response.output_item.added (message)      -> TextStartPart{id=item_index}
//	response.output_text.delta                -> TextDeltaPart{id, delta}
//	response.output_item.done (message)       -> TextEndPart{id}
//	response.output_item.added (function_call)-> ToolInputStartPart{id=call_id, tool_name}
//	response.function_call_arguments.delta    -> ToolInputDeltaPart{id=call_id, delta}
//	response.output_item.done (function_call) -> ToolInputEndPart{id} + ToolCallPart_{...}
//	response.completed                        -> FinishPart{reason, usage, message}
//	error                                     -> ErrorPart + FinishPart{reason: error}
//
// OpenAI Responses stop reasons:
//
//	completed   -> stop
//	incomplete  -> length
//	failed      -> error
//	cancelled   -> error
//
// ToolChoice mapping:
//
//	ToolChoiceAuto     -> omit (Responses default is "auto")
//	ToolChoiceRequired -> "required"
//	ToolChoiceNone     -> "none"
//	ToolChoiceTool     -> {"type":"function","name":"<tool>"}
//
// Reasoning (OpenAI o-series):
//
//	Capabilities.Thinking.Effort -> reasoning.effort ("low"/"medium"/"high"/"max")
//	                                reasoning.summary = "auto"
//	                                include: ["reasoning.encrypted_content"]
//	nil Thinking on a reasoning model -> reasoning.effort = "none"
//
// The Responses API is stateless: we send the full message history on every
// turn (store: false). Reasoning items round-trip via ReasoningPart.Signature
// which stores the raw ResponseReasoningItem JSON.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/wingmodels"
	"github.com/chaserensberger/wingman/wingmodels/catalog"
	provider "github.com/chaserensberger/wingman/wingmodels/providers"
	"github.com/chaserensberger/wingman/wingmodels/transform"
)

// Meta is the registry entry for the OpenAI provider.
var Meta = provider.ProviderMeta{
	ID:        "openai",
	Name:      "OpenAI",
	AuthTypes: []provider.AuthType{provider.AuthTypeAPIKey},
	Factory: func(opts map[string]any) (wingmodels.Model, error) {
		return New(Config{Options: opts})
	},
}

func init() { provider.Register(Meta) }

// Config controls construction of a Client.
type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	MaxTokens  int
	MaxRetries int
	Options    map[string]any
}

const (
	defaultModel      = "gpt-4o"
	defaultMaxTokens  = 4096
	defaultMaxRetries = 3
	defaultBaseURL    = "https://api.openai.com/v1/responses"
	httpTimeout       = 5 * time.Minute
	maxRetryDelay     = 60 * time.Second
)

// Client is a configured OpenAI Model bound to one model id.
type Client struct {
	apiKey     string
	model      string
	baseURL    string
	maxTokens  int
	httpClient *http.Client
	maxRetries int
}

// New constructs a Client. API key resolution order: Config.APIKey,
// Options["api_key"], OPENAI_API_KEY env var.
func New(cfg ...Config) (*Client, error) {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}

	apiKey := c.APIKey
	if apiKey == "" {
		if k, ok := c.Options["api_key"].(string); ok && k != "" {
			apiKey = k
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("openai: no API key (set Config.APIKey, Options[\"api_key\"], or OPENAI_API_KEY)")
	}

	model := c.Model
	if model == "" {
		if m, ok := c.Options["model"].(string); ok && m != "" {
			model = m
		}
	}
	if model == "" {
		model = defaultModel
	}

	baseURL := c.BaseURL
	if baseURL == "" {
		if u, ok := c.Options["base_url"].(string); ok && u != "" {
			baseURL = u
		}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		if v, ok := c.Options["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				maxTokens = n
			case float64:
				maxTokens = int(n)
			}
		}
	}
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	maxRetries := c.MaxRetries
	if maxRetries == 0 {
		if v, ok := c.Options["max_retries"]; ok {
			switch n := v.(type) {
			case int:
				if n >= 0 {
					maxRetries = n
				}
			case float64:
				if int(n) >= 0 {
					maxRetries = int(n)
				}
			}
		}
	}
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	return &Client{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		maxTokens:  maxTokens,
		httpClient: &http.Client{Timeout: httpTimeout},
		maxRetries: maxRetries,
	}, nil
}

// Info returns catalog ModelInfo. API is always stamped to APIOpenAIResponses.
func (c *Client) Info() wingmodels.ModelInfo {
	if info, ok := catalog.Get("openai", c.model); ok {
		info.API = wingmodels.APIOpenAIResponses
		info.BaseURL = c.baseURL
		return info
	}
	return wingmodels.ModelInfo{
		Provider: "openai",
		ID:       c.model,
		API:      wingmodels.APIOpenAIResponses,
		BaseURL:  c.baseURL,
	}
}

func (c *Client) origin() *wingmodels.MessageOrigin {
	return &wingmodels.MessageOrigin{
		Provider: "openai",
		API:      wingmodels.APIOpenAIResponses,
		ModelID:  c.model,
	}
}

// CountTokens returns a char-based approximation (4 chars ≈ 1 token).
// OpenAI does not expose a free token-counting endpoint for the Responses API.
func (c *Client) CountTokens(_ context.Context, msgs []wingmodels.Message) (int, error) {
	total := 0
	for _, m := range msgs {
		for _, p := range m.Content {
			switch v := p.(type) {
			case wingmodels.TextPart:
				total += len(v.Text)
			case wingmodels.ReasoningPart:
				total += len(v.Reasoning)
			case wingmodels.ToolCallPart:
				total += len(v.Name) + 8
				if b, err := json.Marshal(v.Input); err == nil {
					total += len(b)
				}
			case wingmodels.ToolResultPart:
				for _, rp := range v.Output {
					if t, ok := rp.(wingmodels.TextPart); ok {
						total += len(t.Text)
					}
				}
			}
		}
	}
	return total / 4, nil
}

// Stream starts a streaming Responses API request.
func (c *Client) Stream(ctx context.Context, req wingmodels.Request) (*wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], error) {
	info := c.Info()
	req.Messages = transform.Apply(req.Messages, transform.Target{
		Provider:     "openai",
		API:          wingmodels.APIOpenAIResponses,
		ModelID:      c.model,
		Capabilities: info.Capabilities,
	})

	built := c.buildRequest(req, info.Capabilities)
	body, err := json.Marshal(built)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	resp, err := c.doStreamWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](64)
	go runStream(ctx, resp, out, c.origin())
	return out, nil
}

// ---- request building ------------------------------------------------------

// inputItem is the union type for Responses API input items.
// We use map[string]any to avoid a proliferation of structs for items we
// construct exactly once (user/assistant/tool-result messages).
type inputItem = map[string]any

type responsesRequest struct {
	Model          string      `json:"model"`
	Input          []inputItem `json:"input"`
	Stream         bool        `json:"stream"`
	Store          bool        `json:"store"` // always false
	MaxOutputTokens int        `json:"max_output_tokens,omitempty"`
	Tools          []rTool     `json:"tools,omitempty"`
	ToolChoice     any         `json:"tool_choice,omitempty"`
	Reasoning      *rReasoning `json:"reasoning,omitempty"`
	Include        []string    `json:"include,omitempty"`
}

type rTool struct {
	Type        string         `json:"type"` // always "function"
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type rReasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary,omitempty"`
}

func (c *Client) buildRequest(req wingmodels.Request, caps wingmodels.ModelCapabilities) responsesRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = c.maxTokens
	}

	r := responsesRequest{
		Model:           c.model,
		Input:           c.toInputItems(req),
		Stream:          true,
		Store:           false,
		MaxOutputTokens: maxOut,
	}

	// Tools
	if len(req.Tools) > 0 {
		r.Tools = make([]rTool, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i] = rTool{
				Type:        "function",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}
		}
	}

	// ToolChoice (only when tools are present)
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case wingmodels.ToolChoiceRequired:
			r.ToolChoice = "required"
		case wingmodels.ToolChoiceNone:
			r.ToolChoice = "none"
		case wingmodels.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = map[string]any{
					"type": "function",
					"name": req.ToolChoice.Tool,
				}
			}
		// ToolChoiceAuto and zero-value: omit — Responses default is "auto".
		}
	}

	// Reasoning (o-series models)
	if caps.Reasoning {
		if th := req.Capabilities.Thinking; th != nil {
			effort := th.Effort
			if effort == "" {
				effort = "medium"
			}
			r.Reasoning = &rReasoning{Effort: effort, Summary: "auto"}
			r.Include = []string{"reasoning.encrypted_content"}
		} else {
			// Reasoning model but caller didn't ask for it: disable.
			r.Reasoning = &rReasoning{Effort: "none"}
		}
	}

	return r
}

// toInputItems converts a wingmodels message slice to Responses API input items.
//
// Role mapping:
//   - user     -> {role:"user", content:[{type:"input_text",text},...]}
//   - assistant -> multiple top-level items per content block
//     TextPart       -> {type:"message",role:"assistant",id,content:[{type:"output_text",text}],status:"completed"}
//     ReasoningPart  -> restored ReasoningItem (from Signature JSON) or dropped if no sig
//     ToolCallPart   -> {type:"function_call",call_id,name,arguments}
//   - tool     -> {type:"function_call_output",call_id,output}
//
// System prompt is injected as the first item.
func (c *Client) toInputItems(req wingmodels.Request) []inputItem {
	var items []inputItem

	if req.System != "" {
		items = append(items, inputItem{
			"role":    "system",
			"content": req.System,
		})
	}

	msgIdx := 0
	for _, m := range req.Messages {
		switch m.Role {
		case wingmodels.RoleUser:
			var content []map[string]any
			for _, p := range m.Content {
				switch v := p.(type) {
				case wingmodels.TextPart:
					if strings.TrimSpace(v.Text) == "" {
						continue
					}
					content = append(content, map[string]any{
						"type": "input_text",
						"text": v.Text,
					})
				case wingmodels.ImagePart:
					content = append(content, map[string]any{
						"type":      "input_image",
						"detail":    "auto",
						"image_url": "data:" + v.MimeType + ";base64," + v.Data,
					})
				}
			}
			if len(content) > 0 {
				items = append(items, inputItem{
					"role":    "user",
					"content": content,
				})
			}

		case wingmodels.RoleAssistant:
			for _, p := range m.Content {
				switch v := p.(type) {
				case wingmodels.TextPart:
					msgID := "msg_" + strconv.Itoa(msgIdx)
					items = append(items, inputItem{
						"type":   "message",
						"role":   "assistant",
						"id":     msgID,
						"status": "completed",
						"content": []map[string]any{
							{"type": "output_text", "text": v.Text, "annotations": []any{}},
						},
					})
				case wingmodels.ReasoningPart:
					if v.Signature == "" {
						// No round-trip signature; drop — can't replay without it.
						continue
					}
					// Signature stores the raw ResponseReasoningItem JSON.
					var reasoningItem map[string]any
					if err := json.Unmarshal([]byte(v.Signature), &reasoningItem); err != nil {
						continue
					}
					items = append(items, reasoningItem)
				case wingmodels.ToolCallPart:
					callID, itemID := splitCompositeID(v.CallID)
					args, _ := json.Marshal(v.Input)
					item := inputItem{
						"type":      "function_call",
						"call_id":   callID,
						"name":      v.Name,
						"arguments": string(args),
					}
					if itemID != "" {
						item["id"] = itemID
					}
					items = append(items, item)
				}
			}

		case wingmodels.RoleTool:
			for _, p := range m.Content {
				if tr, ok := p.(wingmodels.ToolResultPart); ok {
					callID, _ := splitCompositeID(tr.CallID)
					output := toolResultOutput(tr.Output)
					items = append(items, inputItem{
						"type":    "function_call_output",
						"call_id": callID,
						"output":  output,
					})
				}
			}
		}
		msgIdx++
	}
	return items
}

// splitCompositeID splits a "callID|itemID" composite. If there's no "|",
// returns (id, "").
func splitCompositeID(id string) (callID, itemID string) {
	if i := strings.IndexByte(id, '|'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

// toolResultOutput converts a ToolResultPart output slice to a string or
// content-array for the Responses API.
func toolResultOutput(out []wingmodels.Part) any {
	if len(out) == 0 {
		return ""
	}
	// Fast path: single text → plain string.
	if len(out) == 1 {
		if t, ok := out[0].(wingmodels.TextPart); ok {
			return t.Text
		}
	}
	var parts []map[string]any
	for _, p := range out {
		switch v := p.(type) {
		case wingmodels.TextPart:
			parts = append(parts, map[string]any{"type": "input_text", "text": v.Text})
		case wingmodels.ImagePart:
			parts = append(parts, map[string]any{
				"type":      "input_image",
				"detail":    "auto",
				"image_url": "data:" + v.MimeType + ";base64," + v.Data,
			})
		}
	}
	return parts
}

// ---- stream parsing --------------------------------------------------------

// streamParser holds in-flight assembly state for one Responses stream.
type streamParser struct {
	out        *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]
	terminated bool
	origin     *wingmodels.MessageOrigin

	// per-item state keyed by item index (order of output_item.added events)
	items []*responseItem

	// Final assembly
	content      wingmodels.Content
	usage        wingmodels.Usage
	stopReason   string
	responseMeta wingmodels.ResponseMetadata
	startEmitted bool
}

type itemKind int

const (
	itemText itemKind = iota
	itemReasoning
	itemFunctionCall
)

type responseItem struct {
	kind     itemKind
	id       string // wingmodels stream-part id
	callID   string // function_call.call_id (Responses API)
	itemID   string // function_call.id (Responses API)
	toolName string
	textBuf  strings.Builder
	jsonBuf  strings.Builder
	// Reasoning: accumulate the full ResponseReasoningItem JSON for round-trip.
	// We build it from the summary deltas and store it in ReasoningPart.Signature.
	reasoningJSON strings.Builder
}

func newStreamParser(out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], origin *wingmodels.MessageOrigin) *streamParser {
	return &streamParser{out: out, origin: origin}
}

// handle dispatches one SSE event. Returns true when the stream is terminated.
func (p *streamParser) handle(eventType, data string) bool {
	switch eventType {
	case "response.created":
		var ev struct {
			Response struct {
				ID string `json:"id"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("openai: parse response.created: %w", err))
			return true
		}
		p.responseMeta = wingmodels.ResponseMetadata{ID: ev.Response.ID}
		p.out.Push(wingmodels.StreamStartPart{})
		p.startEmitted = true
		p.out.Push(wingmodels.ResponseMetadataPart{ResponseMetadata: p.responseMeta})

	case "response.output_item.added":
		var ev struct {
			Item struct {
				Type   string `json:"type"`
				ID     string `json:"id"`
				Name   string `json:"name"`
				CallID string `json:"call_id"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("openai: parse output_item.added: %w", err))
			return true
		}
		idx := len(p.items)
		id := itemID(idx)
		switch ev.Item.Type {
		case "reasoning":
			item := &responseItem{kind: itemReasoning, id: id}
			p.items = append(p.items, item)
			p.out.Push(wingmodels.ReasoningStartPart{ID: id})
		case "message":
			item := &responseItem{kind: itemText, id: id}
			p.items = append(p.items, item)
			p.out.Push(wingmodels.TextStartPart{ID: id})
		case "function_call":
			item := &responseItem{
				kind:     itemFunctionCall,
				id:       id,
				callID:   ev.Item.CallID,
				itemID:   ev.Item.ID,
				toolName: ev.Item.Name,
			}
			p.items = append(p.items, item)
			p.out.Push(wingmodels.ToolInputStartPart{ID: ev.Item.CallID, ToolName: ev.Item.Name})
		default:
			p.items = append(p.items, &responseItem{id: id}) // placeholder
		}

	case "response.reasoning_summary_text.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false
		}
		item := p.currentReasoningItem()
		if item == nil {
			return false
		}
		item.textBuf.WriteString(ev.Delta)
		p.out.Push(wingmodels.ReasoningDeltaPart{ID: item.id, Delta: ev.Delta})

	case "response.output_text.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false
		}
		item := p.currentTextItem()
		if item == nil {
			return false
		}
		item.textBuf.WriteString(ev.Delta)
		p.out.Push(wingmodels.TextDeltaPart{ID: item.id, Delta: ev.Delta})

	case "response.refusal.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false
		}
		item := p.currentTextItem()
		if item == nil {
			return false
		}
		item.textBuf.WriteString(ev.Delta)
		p.out.Push(wingmodels.TextDeltaPart{ID: item.id, Delta: ev.Delta})

	case "response.function_call_arguments.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false
		}
		item := p.currentFunctionCallItem()
		if item == nil {
			return false
		}
		item.jsonBuf.WriteString(ev.Delta)
		p.out.Push(wingmodels.ToolInputDeltaPart{ID: item.callID, Delta: ev.Delta})

	case "response.output_item.done":
		var ev struct {
			Item json.RawMessage `json:"item"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false
		}
		var itemType struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(ev.Item, &itemType); err != nil {
			return false
		}
		item := p.lastItem()
		if item == nil {
			return false
		}
		switch itemType.Type {
		case "reasoning":
			// Store the full item JSON as the round-trip signature.
			p.content = append(p.content, wingmodels.ReasoningPart{
				Reasoning: item.textBuf.String(),
				Signature: string(ev.Item),
			})
			p.out.Push(wingmodels.ReasoningEndPart{ID: item.id})

		case "message":
			p.content = append(p.content, wingmodels.TextPart{Text: item.textBuf.String()})
			p.out.Push(wingmodels.TextEndPart{ID: item.id})

		case "function_call":
			var fullItem struct {
				Arguments string `json:"arguments"`
				CallID    string `json:"call_id"`
				ID        string `json:"id"`
				Name      string `json:"name"`
			}
			_ = json.Unmarshal(ev.Item, &fullItem)

			// Finalize args from accumulated buffer or done event.
			argJSON := item.jsonBuf.String()
			if argJSON == "" {
				argJSON = fullItem.Arguments
			}
			var input map[string]any
			if argJSON != "" {
				_ = json.Unmarshal([]byte(argJSON), &input)
			}

			callID := item.callID
			itemIDStr := item.itemID
			if callID == "" {
				callID = fullItem.CallID
			}
			if itemIDStr == "" {
				itemIDStr = fullItem.ID
			}
			compositeID := callID
			if itemIDStr != "" {
				compositeID = callID + "|" + itemIDStr
			}

			p.content = append(p.content, wingmodels.ToolCallPart{
				CallID: compositeID,
				Name:   item.toolName,
				Input:  input,
			})
			p.out.Push(wingmodels.ToolInputEndPart{ID: callID})
			p.out.Push(wingmodels.ToolCallPart_{ID: callID, ToolName: item.toolName, Input: input})
		}

	case "response.completed":
		var ev struct {
			Response struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Usage  struct {
					InputTokens        int `json:"input_tokens"`
					OutputTokens       int `json:"output_tokens"`
					TotalTokens        int `json:"total_tokens"`
					InputTokensDetails struct {
						CachedTokens int `json:"cached_tokens"`
					} `json:"input_tokens_details"`
				} `json:"usage"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("openai: parse response.completed: %w", err))
			return true
		}
		p.responseMeta.ID = ev.Response.ID
		cached := ev.Response.Usage.InputTokensDetails.CachedTokens
		p.usage = wingmodels.Usage{
			InputTokens:       ev.Response.Usage.InputTokens - cached,
			OutputTokens:      ev.Response.Usage.OutputTokens,
			TotalTokens:       ev.Response.Usage.TotalTokens,
			CachedInputTokens: cached,
		}
		p.stopReason = ev.Response.Status
		p.terminateNormal()
		return true

	case "response.failed":
		var ev struct {
			Response struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
				IncompleteDetails struct {
					Reason string `json:"reason"`
				} `json:"incomplete_details"`
			} `json:"response"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		msg := ev.Response.Error.Message
		if msg == "" {
			msg = ev.Response.IncompleteDetails.Reason
		}
		if msg == "" {
			msg = "unknown error"
		}
		p.out.Push(wingmodels.ErrorPart{Message: msg, Code: ev.Response.Error.Code})
		p.terminate(wingmodels.FinishReasonError, fmt.Errorf("openai: %s", msg))
		return true

	case "error":
		var ev struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		p.out.Push(wingmodels.ErrorPart{Message: ev.Message, Code: ev.Code})
		p.terminate(wingmodels.FinishReasonError, fmt.Errorf("openai: %s: %s", ev.Code, ev.Message))
		return true
	}
	return false
}

func (p *streamParser) currentReasoningItem() *responseItem {
	for i := len(p.items) - 1; i >= 0; i-- {
		if p.items[i].kind == itemReasoning {
			return p.items[i]
		}
	}
	return nil
}

func (p *streamParser) currentTextItem() *responseItem {
	for i := len(p.items) - 1; i >= 0; i-- {
		if p.items[i].kind == itemText {
			return p.items[i]
		}
	}
	return nil
}

func (p *streamParser) currentFunctionCallItem() *responseItem {
	for i := len(p.items) - 1; i >= 0; i-- {
		if p.items[i].kind == itemFunctionCall {
			return p.items[i]
		}
	}
	return nil
}

func (p *streamParser) lastItem() *responseItem {
	if len(p.items) == 0 {
		return nil
	}
	return p.items[len(p.items)-1]
}

func (p *streamParser) terminateNormal() {
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens + p.usage.CachedInputTokens
	reason := mapStopReason(p.stopReason)
	msg := &wingmodels.Message{
		Role:         wingmodels.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: reason,
	}
	p.out.Push(wingmodels.FinishPart{
		Reason:   reason,
		Usage:    p.usage,
		Message:  msg,
		Metadata: p.responseMeta,
	})
	p.close(msg, nil)
}

func (p *streamParser) terminateError(err error) {
	if p.terminated {
		return
	}
	if !p.startEmitted {
		p.out.Push(wingmodels.StreamStartPart{})
	}
	p.out.Push(wingmodels.ErrorPart{Message: err.Error()})
	p.terminate(wingmodels.FinishReasonError, err)
}

func (p *streamParser) terminateAborted() {
	if p.terminated {
		return
	}
	p.terminate(wingmodels.FinishReasonAborted, context.Canceled)
}

func (p *streamParser) terminate(reason wingmodels.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
	msg := &wingmodels.Message{
		Role:         wingmodels.RoleAssistant,
		Content:      p.content,
		Origin:       p.origin,
		FinishReason: reason,
	}
	p.out.Push(wingmodels.FinishPart{
		Reason:   reason,
		Usage:    p.usage,
		Message:  msg,
		Metadata: p.responseMeta,
	})
	p.close(msg, err)
}

func (p *streamParser) close(msg *wingmodels.Message, err error) {
	p.terminated = true
	p.out.Close(msg, err)
}

func itemID(index int) string { return "item_" + strconv.Itoa(index) }

func mapStopReason(status string) wingmodels.FinishReason {
	switch status {
	case "completed":
		return wingmodels.FinishReasonStop
	case "incomplete":
		return wingmodels.FinishReasonLength
	case "failed", "cancelled":
		return wingmodels.FinishReasonError
	case "":
		return wingmodels.FinishReasonUnknown
	default:
		return wingmodels.FinishReasonOther
	}
}

// runStream drives the SSE scanner. Emits events on out; closes it exactly once.
func runStream(ctx context.Context, resp *http.Response, out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], origin *wingmodels.MessageOrigin) {
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	p := newStreamParser(out, origin)

	var eventType string
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			p.terminateAborted()
			return
		}
		line := scanner.Text()
		switch {
		case line == "":
			// SSE event boundary.
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			if done := p.handle(eventType, data); done {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		p.terminateError(fmt.Errorf("openai: scanner: %w", err))
		return
	}
	p.terminateError(fmt.Errorf("openai: stream closed without response.completed"))
}

// ---- HTTP plumbing ---------------------------------------------------------

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func (c *Client) doStreamWithRetry(ctx context.Context, body []byte) (*http.Response, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.doRequest(ctx, body)
		if err != nil {
			if shouldRetryTransport(err) && attempt < c.maxRetries {
				if waitErr := waitForRetry(ctx, backoffDelay(attempt)); waitErr != nil {
					return nil, fmt.Errorf("openai: %w", waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("openai: request: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if shouldRetryStatus(resp.StatusCode) && attempt < c.maxRetries {
			if waitErr := waitForRetry(ctx, retryDelay(resp, attempt)); waitErr != nil {
				return nil, fmt.Errorf("openai: API %d: %s", resp.StatusCode, string(raw))
			}
			continue
		}
		return nil, fmt.Errorf("openai: API %d: %s", resp.StatusCode, string(raw))
	}
	return nil, fmt.Errorf("openai: retries exhausted")
}

func (c *Client) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.httpClient.Do(req)
}

func shouldRetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

func shouldRetryTransport(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	return true
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return d
		}
	}
	return backoffDelay(attempt)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	if seconds, err := time.ParseDuration(v + "s"); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > maxRetryDelay {
			seconds = maxRetryDelay
		}
		return seconds, true
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, false
		}
		if d > maxRetryDelay {
			d = maxRetryDelay
		}
		return d, true
	}
	return 0, false
}

func backoffDelay(attempt int) time.Duration {
	d := time.Second * time.Duration(2<<attempt)
	if d > maxRetryDelay {
		return maxRetryDelay
	}
	return d
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Compile-time check.
var _ wingmodels.Model = (*Client)(nil)
