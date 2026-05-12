// Package openairesponses implements OpenAI's Responses API protocol.
package openairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols/internal/shared"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/models/transform"
)

// Protocol adapts models.Request to the OpenAI Responses API.
type Protocol struct{}

func (Protocol) API() models.API { return models.APIOpenAIResponses }

func (Protocol) Prepare(_ context.Context, ref route.ModelRef, req models.Request) (*route.PreparedBody, error) {
	req.Messages = transform.Apply(req.Messages, transform.Target{Provider: ref.Provider, API: models.APIOpenAIResponses, ModelID: ref.ModelID, Capabilities: ref.Info.Capabilities})
	body, err := json.Marshal(buildRequest(ref, req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal responses request: %w", ref.Provider, err)
	}
	return &route.PreparedBody{Body: body}, nil
}

func (Protocol) ParseStream(ctx context.Context, ref route.ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message]) {
	p := newParser(out, shared.Origin(ref.Provider, models.APIOpenAIResponses, ref.ModelID), ref.Provider)
	shared.ScanSSE(ctx, resp, p.handle, p.terminateError, ref.Provider+": stream closed without response.completed")
}

func (Protocol) CountTokens(_ context.Context, _ route.ModelRef, msgs []models.Message) (int, error) {
	return shared.CountTokens(msgs), nil
}

type responsesRequest struct {
	Model           string      `json:"model"`
	Input           []inputItem `json:"input"`
	Stream          bool        `json:"stream"`
	Store           bool        `json:"store"`
	MaxOutputTokens int         `json:"max_output_tokens,omitempty"`
	Tools           []rTool     `json:"tools,omitempty"`
	ToolChoice      any         `json:"tool_choice,omitempty"`
	Reasoning       *rReasoning `json:"reasoning,omitempty"`
	Include         []string    `json:"include,omitempty"`
	Text            *rText      `json:"text,omitempty"`
}

type inputItem = map[string]any

type rTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type rReasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary,omitempty"`
}

type rText struct {
	Format *rTextFormat `json:"format,omitempty"`
}

type rTextFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict,omitempty"`
}

func buildRequest(ref route.ModelRef, req models.Request) responsesRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = ref.MaxOutputTokens
	}
	r := responsesRequest{Model: ref.ModelID, Input: toInputItems(req), Stream: true, Store: false, MaxOutputTokens: maxOut}
	if len(req.Tools) > 0 {
		r.Tools = make([]rTool, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i] = rTool{Type: "function", Name: t.Name, Description: t.Description, Parameters: t.InputSchema}
		}
	}
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case models.ToolChoiceRequired:
			r.ToolChoice = "required"
		case models.ToolChoiceNone:
			r.ToolChoice = "none"
		case models.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = map[string]any{"type": "function", "name": req.ToolChoice.Tool}
			}
		}
	}
	if req.OutputSchema != nil {
		name := req.OutputSchema.Name
		if name == "" {
			name = "response"
		}
		r.Text = &rText{Format: &rTextFormat{Type: "json_schema", Name: name, Schema: req.OutputSchema.Schema, Strict: req.OutputSchema.Strict}}
	}
	if ref.Info.Capabilities.Reasoning {
		if th := req.Capabilities.Thinking; th != nil {
			effort := th.Effort
			if effort == "" {
				effort = "medium"
			}
			r.Reasoning = &rReasoning{Effort: effort, Summary: "auto"}
			r.Include = []string{"reasoning.encrypted_content"}
		} else {
			r.Reasoning = &rReasoning{Effort: "none"}
		}
	}
	return r
}

func toInputItems(req models.Request) []inputItem {
	var items []inputItem
	if req.System != "" {
		items = append(items, inputItem{"role": "system", "content": req.System})
	}
	msgIdx := 0
	for _, m := range req.Messages {
		switch m.Role {
		case models.RoleUser:
			var content []map[string]any
			for _, p := range m.Content {
				switch v := p.(type) {
				case models.TextPart:
					if strings.TrimSpace(v.Text) != "" {
						content = append(content, map[string]any{"type": "input_text", "text": v.Text})
					}
				case models.ImagePart:
					content = append(content, map[string]any{"type": "input_image", "detail": "auto", "image_url": "data:" + v.MimeType + ";base64," + v.Data})
				}
			}
			if len(content) > 0 {
				items = append(items, inputItem{"role": "user", "content": content})
			}
		case models.RoleAssistant:
			for _, p := range m.Content {
				switch v := p.(type) {
				case models.TextPart:
					items = append(items, inputItem{"type": "message", "role": "assistant", "id": "msg_" + strconv.Itoa(msgIdx), "status": "completed", "content": []map[string]any{{"type": "output_text", "text": v.Text, "annotations": []any{}}}})
				case models.ReasoningPart:
					if v.Signature != "" {
						var item map[string]any
						if json.Unmarshal([]byte(v.Signature), &item) == nil {
							items = append(items, item)
						}
					}
				case models.ToolCallPart:
					callID, itemID := shared.SplitCompositeID(v.CallID)
					args, _ := json.Marshal(v.Input)
					item := inputItem{"type": "function_call", "call_id": callID, "name": v.Name, "arguments": string(args)}
					if itemID != "" {
						item["id"] = itemID
					}
					items = append(items, item)
				}
			}
		case models.RoleTool:
			for _, p := range m.Content {
				if tr, ok := p.(models.ToolResultPart); ok {
					callID, _ := shared.SplitCompositeID(tr.CallID)
					items = append(items, inputItem{"type": "function_call_output", "call_id": callID, "output": shared.ToolResultText(tr.Output)})
				}
			}
		}
		msgIdx++
	}
	return items
}

type parser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	terminated bool
	origin     *models.MessageOrigin
	provider   string
	items      []*item
	content    models.Content
	usage      models.Usage
	meta       models.ResponseMetadata
}

type item struct {
	kind     string
	id       string
	callID   string
	itemID   string
	toolName string
	textBuf  strings.Builder
	jsonBuf  strings.Builder
}

func newParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, provider string) *parser {
	return &parser{out: out, origin: origin, provider: provider}
}

func (p *parser) handle(eventType, data string) bool {
	switch eventType {
	case "response.created":
		var ev struct {
			Response struct {
				ID string `json:"id"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(err)
			return true
		}
		p.meta.ID = ev.Response.ID
		p.out.Push(models.StreamStartPart{})
		p.out.Push(models.ResponseMetadataPart{ResponseMetadata: p.meta})
	case "response.output_item.added":
		var ev struct {
			Item struct{ Type, ID, Name, CallID string } `json:"item"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return false
		}
		id := "item_" + strconv.Itoa(len(p.items))
		it := &item{kind: ev.Item.Type, id: id, callID: ev.Item.CallID, itemID: ev.Item.ID, toolName: ev.Item.Name}
		p.items = append(p.items, it)
		switch ev.Item.Type {
		case "reasoning":
			p.out.Push(models.ReasoningStartPart{ID: id})
		case "message":
			p.out.Push(models.TextStartPart{ID: id})
		case "function_call":
			p.out.Push(models.ToolInputStartPart{ID: ev.Item.CallID, ToolName: ev.Item.Name})
		}
	case "response.reasoning_summary_text.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &ev) == nil {
			if it := p.current("reasoning"); it != nil {
				it.textBuf.WriteString(ev.Delta)
				p.out.Push(models.ReasoningDeltaPart{ID: it.id, Delta: ev.Delta})
			}
		}
	case "response.output_text.delta", "response.refusal.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &ev) == nil {
			if it := p.current("message"); it != nil {
				it.textBuf.WriteString(ev.Delta)
				p.out.Push(models.TextDeltaPart{ID: it.id, Delta: ev.Delta})
			}
		}
	case "response.function_call_arguments.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &ev) == nil {
			if it := p.current("function_call"); it != nil {
				it.jsonBuf.WriteString(ev.Delta)
				p.out.Push(models.ToolInputDeltaPart{ID: it.callID, Delta: ev.Delta})
			}
		}
	case "response.output_item.done":
		p.finishLastItem(data)
	case "response.completed", "response.incomplete":
		p.handleCompleted(data, eventType)
		return true
	case "response.failed", "error":
		p.out.Push(models.ErrorPart{Message: p.errorMessage(data)})
		p.terminate(models.FinishReasonError, fmt.Errorf("%s: response failed", p.provider))
		return true
	}
	return false
}

func (p *parser) current(kind string) *item {
	for i := len(p.items) - 1; i >= 0; i-- {
		if p.items[i].kind == kind {
			return p.items[i]
		}
	}
	return nil
}

func (p *parser) finishLastItem(data string) {
	if len(p.items) == 0 {
		return
	}
	it := p.items[len(p.items)-1]
	var ev struct {
		Item json.RawMessage `json:"item"`
	}
	_ = json.Unmarshal([]byte(data), &ev)
	switch it.kind {
	case "reasoning":
		p.content = append(p.content, models.ReasoningPart{Reasoning: it.textBuf.String(), Signature: string(ev.Item)})
		p.out.Push(models.ReasoningEndPart{ID: it.id})
	case "message":
		p.content = append(p.content, models.TextPart{Text: it.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: it.id})
	case "function_call":
		var full struct{ Arguments, CallID, ID, Name string }
		_ = json.Unmarshal(ev.Item, &full)
		args := it.jsonBuf.String()
		if args == "" {
			args = full.Arguments
		}
		var input map[string]any
		_ = json.Unmarshal([]byte(args), &input)
		callID := firstNonEmpty(it.callID, full.CallID)
		itemID := firstNonEmpty(it.itemID, full.ID)
		composite := callID
		if itemID != "" {
			composite += "|" + itemID
		}
		name := firstNonEmpty(it.toolName, full.Name)
		p.content = append(p.content, models.ToolCallPart{CallID: composite, Name: name, Input: input})
		p.out.Push(models.ToolInputEndPart{ID: callID})
		p.out.Push(models.ToolCallPart_{ID: callID, ToolName: name, Input: input})
	}
}

func (p *parser) handleCompleted(data, eventType string) {
	var ev struct {
		Response struct {
			ID                string `json:"id"`
			IncompleteDetails *struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Usage struct {
				InputTokens        int `json:"input_tokens"`
				OutputTokens       int `json:"output_tokens"`
				TotalTokens        int `json:"total_tokens"`
				InputTokensDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	_ = json.Unmarshal([]byte(data), &ev)
	p.meta.ID = firstNonEmpty(ev.Response.ID, p.meta.ID)
	cached := ev.Response.Usage.InputTokensDetails.CachedTokens
	p.usage = models.Usage{InputTokens: ev.Response.Usage.InputTokens - cached, OutputTokens: ev.Response.Usage.OutputTokens, TotalTokens: ev.Response.Usage.TotalTokens, CachedInputTokens: cached}
	reason := models.FinishReasonStop
	if eventType == "response.incomplete" || ev.Response.IncompleteDetails != nil {
		reason = models.FinishReasonLength
	}
	p.terminate(reason, nil)
}

func (p *parser) errorMessage(data string) string {
	var ev struct {
		Message  string `json:"message"`
		Code     string `json:"code"`
		Response struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	_ = json.Unmarshal([]byte(data), &ev)
	if ev.Response.Error.Message != "" {
		return ev.Response.Error.Message
	}
	return firstNonEmpty(ev.Message, ev.Code, "OpenAI Responses stream error")
}

func (p *parser) terminateError(err error) { p.terminate(models.FinishReasonError, err) }

func (p *parser) terminate(reason models.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.terminated = true
	if p.usage.TotalTokens == 0 {
		p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens + p.usage.CachedInputTokens
	}
	msg := &models.Message{Role: models.RoleAssistant, Content: p.content, Origin: p.origin, FinishReason: reason}
	if err != nil {
		p.out.Push(models.ErrorPart{Message: err.Error()})
	}
	p.out.Push(models.FinishPart{Reason: reason, Usage: p.usage, Message: msg, Metadata: p.meta})
	p.out.Close(msg, err)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ route.Protocol = Protocol{}
