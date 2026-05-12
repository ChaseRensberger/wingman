// Package bedrockconverse implements AWS Bedrock's Converse request protocol.
package bedrockconverse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols/internal/shared"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/models/transform"
)

// Protocol adapts models.Request to Bedrock ConverseStream bodies. It expects
// the transport/framing layer to provide decoded JSON event payloads.
type Protocol struct{}

func (Protocol) API() models.API { return models.APIBedrockConverse }

func (Protocol) Prepare(_ context.Context, ref route.ModelRef, req models.Request) (*route.PreparedBody, error) {
	req.Messages = transform.Apply(req.Messages, transform.Target{Provider: ref.Provider, API: models.APIBedrockConverse, ModelID: ref.ModelID, Capabilities: ref.Info.Capabilities})
	body, err := json.Marshal(buildRequest(ref, req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal bedrock converse request: %w", ref.Provider, err)
	}
	return &route.PreparedBody{Body: body}, nil
}

func (Protocol) ParseStream(ctx context.Context, ref route.ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message]) {
	defer resp.Body.Close()
	p := newParser(out, shared.Origin(ref.Provider, models.APIBedrockConverse, ref.ModelID))
	p.out.Push(models.StreamStartPart{})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			p.terminate(models.FinishReasonAborted, err)
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if p.handle([]byte(line)) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		p.terminate(models.FinishReasonError, err)
		return
	}
	p.finish()
}

func (Protocol) CountTokens(_ context.Context, _ route.ModelRef, msgs []models.Message) (int, error) {
	return shared.CountTokens(msgs), nil
}

// Endpoint returns Bedrock's model-specific ConverseStream endpoint.
func Endpoint() route.Endpoint {
	return route.EndpointFunc(func(ref route.ModelRef) (string, error) {
		return route.JoinPath(ref.BaseURL, "/model/"+url.PathEscape(ref.ModelID)+"/converse-stream")
	})
}

type request struct {
	ModelID         string           `json:"modelId"`
	Messages        []message        `json:"messages"`
	System          []block          `json:"system,omitempty"`
	InferenceConfig *inferenceConfig `json:"inferenceConfig,omitempty"`
	ToolConfig      *toolConfig      `json:"toolConfig,omitempty"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type block struct {
	Text             string            `json:"text,omitempty"`
	ToolUse          *toolUse          `json:"toolUse,omitempty"`
	ToolResult       *toolResult       `json:"toolResult,omitempty"`
	ReasoningContent *reasoningContent `json:"reasoningContent,omitempty"`
}

type toolUse struct {
	ToolUseID string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

type toolResult struct {
	ToolUseID string            `json:"toolUseId"`
	Content   []toolResultBlock `json:"content"`
	Status    string            `json:"status,omitempty"`
}

type toolResultBlock struct {
	Text string `json:"text,omitempty"`
}

type reasoningContent struct {
	ReasoningText reasoningText `json:"reasoningText"`
}

type reasoningText struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

type inferenceConfig struct {
	MaxTokens int `json:"maxTokens,omitempty"`
}

type toolConfig struct {
	Tools      []bedrockTool `json:"tools"`
	ToolChoice any           `json:"toolChoice,omitempty"`
}

type bedrockTool struct {
	ToolSpec toolSpec `json:"toolSpec"`
}

type toolSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	JSON map[string]any `json:"json"`
}

func buildRequest(ref route.ModelRef, req models.Request) request {
	out := request{ModelID: ref.ModelID, Messages: toMessages(req.Messages)}
	if req.System != "" {
		out.System = []block{{Text: req.System}}
	}
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = ref.MaxOutputTokens
	}
	if maxOut > 0 {
		out.InferenceConfig = &inferenceConfig{MaxTokens: maxOut}
	}
	if len(req.Tools) > 0 && req.ToolChoice.Mode != models.ToolChoiceNone {
		tools := make([]bedrockTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = bedrockTool{ToolSpec: toolSpec{Name: t.Name, Description: t.Description, InputSchema: inputSchema{JSON: t.InputSchema}}}
		}
		out.ToolConfig = &toolConfig{Tools: tools, ToolChoice: lowerToolChoice(req.ToolChoice)}
	}
	return out
}

func lowerToolChoice(choice models.ToolChoice) any {
	switch choice.Mode {
	case models.ToolChoiceRequired:
		return map[string]any{"any": map[string]any{}}
	case models.ToolChoiceTool:
		if choice.Tool != "" {
			return map[string]any{"tool": map[string]any{"name": choice.Tool}}
		}
	}
	return map[string]any{"auto": map[string]any{}}
}

func toMessages(msgs []models.Message) []message {
	var out []message
	for _, m := range msgs {
		role := string(m.Role)
		if m.Role == models.RoleTool {
			role = "user"
		}
		var content []block
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				if v.Text != "" {
					content = append(content, block{Text: v.Text})
				}
			case models.ReasoningPart:
				content = append(content, block{ReasoningContent: &reasoningContent{ReasoningText: reasoningText{Text: v.Reasoning, Signature: v.Signature}}})
			case models.ToolCallPart:
				content = append(content, block{ToolUse: &toolUse{ToolUseID: v.CallID, Name: v.Name, Input: v.Input}})
			case models.ToolResultPart:
				status := "success"
				if v.IsError {
					status = "error"
				}
				content = append(content, block{ToolResult: &toolResult{ToolUseID: v.CallID, Content: []toolResultBlock{{Text: shared.ToolResultText(v.Output)}}, Status: status}})
			}
		}
		if len(content) > 0 {
			out = append(out, message{Role: role, Content: content})
		}
	}
	return out
}

type parser struct {
	out        *models.EventStream[models.StreamPart, *models.Message]
	origin     *models.MessageOrigin
	content    models.Content
	usage      models.Usage
	reason     models.FinishReason
	textBuf    strings.Builder
	textOpen   bool
	terminated bool
}

func newParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin) *parser {
	return &parser{out: out, origin: origin, reason: models.FinishReasonStop}
}

func (p *parser) handle(data []byte) bool {
	var ev struct {
		ContentBlockDelta *struct {
			ContentBlockIndex int `json:"contentBlockIndex"`
			Delta             struct {
				Text string `json:"text"`
			} `json:"delta"`
		} `json:"contentBlockDelta"`
		MessageStop *struct {
			StopReason string `json:"stopReason"`
		} `json:"messageStop"`
		Metadata *struct {
			Usage struct{ InputTokens, OutputTokens, TotalTokens int } `json:"usage"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		p.terminate(models.FinishReasonError, err)
		return true
	}
	if ev.ContentBlockDelta != nil && ev.ContentBlockDelta.Delta.Text != "" {
		if !p.textOpen {
			p.textOpen = true
			p.out.Push(models.TextStartPart{ID: "text_0"})
		}
		p.textBuf.WriteString(ev.ContentBlockDelta.Delta.Text)
		p.out.Push(models.TextDeltaPart{ID: "text_0", Delta: ev.ContentBlockDelta.Delta.Text})
	}
	if ev.MessageStop != nil {
		p.reason = mapStopReason(ev.MessageStop.StopReason)
	}
	if ev.Metadata != nil {
		p.usage = models.Usage{InputTokens: ev.Metadata.Usage.InputTokens, OutputTokens: ev.Metadata.Usage.OutputTokens, TotalTokens: ev.Metadata.Usage.TotalTokens}
		p.finish()
		return true
	}
	return false
}

func mapStopReason(reason string) models.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence", "":
		return models.FinishReasonStop
	case "max_tokens":
		return models.FinishReasonLength
	case "tool_use":
		return models.FinishReasonToolCalls
	case "content_filtered", "guardrail_intervened":
		return models.FinishReasonContentFilter
	default:
		return models.FinishReasonOther
	}
}

func (p *parser) finish() { p.terminate(p.reason, nil) }

func (p *parser) terminate(reason models.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.terminated = true
	if p.textOpen {
		p.content = append(p.content, models.TextPart{Text: p.textBuf.String()})
		p.out.Push(models.TextEndPart{ID: "text_0"})
	}
	msg := &models.Message{Role: models.RoleAssistant, Content: p.content, Origin: p.origin, FinishReason: reason}
	if err != nil {
		p.out.Push(models.ErrorPart{Message: err.Error()})
	}
	p.out.Push(models.FinishPart{Reason: reason, Usage: p.usage, Message: msg})
	p.out.Close(msg, err)
}

var _ route.Protocol = Protocol{}
