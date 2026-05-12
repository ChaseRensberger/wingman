// Package gemini implements Google's Gemini GenerateContent protocol.
package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols/internal/shared"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/models/transform"
)

// Protocol adapts models.Request to Gemini streamGenerateContent.
type Protocol struct{}

func (Protocol) API() models.API { return models.APIGoogleGenAI }

func (Protocol) Prepare(_ context.Context, ref route.ModelRef, req models.Request) (*route.PreparedBody, error) {
	req.Messages = transform.Apply(req.Messages, transform.Target{Provider: ref.Provider, API: models.APIGoogleGenAI, ModelID: ref.ModelID, Capabilities: ref.Info.Capabilities})
	body, err := json.Marshal(buildRequest(ref, req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal gemini request: %w", ref.Provider, err)
	}
	return &route.PreparedBody{Body: body}, nil
}

func (Protocol) ParseStream(ctx context.Context, ref route.ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message]) {
	defer resp.Body.Close()
	p := newParser(out, shared.Origin(ref.Provider, models.APIGoogleGenAI, ref.ModelID))
	p.out.Push(models.StreamStartPart{})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			p.terminate(models.FinishReasonAborted, err)
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if p.handle(strings.TrimPrefix(line, "data: ")) {
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

// Endpoint returns Gemini's model-specific SSE endpoint.
func Endpoint() route.Endpoint {
	return route.EndpointFunc(func(ref route.ModelRef) (string, error) {
		base := strings.TrimRight(ref.BaseURL, "/")
		if base == "" {
			return "", fmt.Errorf("gemini endpoint: missing base URL")
		}
		return base + "/models/" + url.PathEscape(ref.ModelID) + ":streamGenerateContent?alt=sse", nil
	})
}

type request struct {
	Contents          []content          `json:"contents"`
	SystemInstruction *systemInstruction `json:"systemInstruction,omitempty"`
	Tools             []tool             `json:"tools,omitempty"`
	ToolConfig        *toolConfig        `json:"toolConfig,omitempty"`
	GenerationConfig  *generationConfig  `json:"generationConfig,omitempty"`
}

type systemInstruction struct {
	Parts []part `json:"parts"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type functionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type functionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type tool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type functionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolConfig struct {
	FunctionCallingConfig functionCallingConfig `json:"functionCallingConfig"`
}

type functionCallingConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type generationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	ThinkingBudget  int  `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

func buildRequest(ref route.ModelRef, req models.Request) request {
	out := request{Contents: toContents(req.Messages)}
	if req.System != "" {
		out.SystemInstruction = &systemInstruction{Parts: []part{{Text: req.System}}}
	}
	if len(req.Tools) > 0 && req.ToolChoice.Mode != models.ToolChoiceNone {
		decls := make([]functionDeclaration, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = functionDeclaration{Name: t.Name, Description: t.Description, Parameters: t.InputSchema}
		}
		out.Tools = []tool{{FunctionDeclarations: decls}}
		out.ToolConfig = lowerToolConfig(req.ToolChoice)
	}
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = ref.MaxOutputTokens
	}
	if maxOut > 0 || req.Capabilities.Thinking != nil {
		out.GenerationConfig = &generationConfig{MaxOutputTokens: maxOut}
		if th := req.Capabilities.Thinking; th != nil {
			out.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingBudget: th.BudgetTokens, IncludeThoughts: true}
		}
	}
	return out
}

func lowerToolConfig(choice models.ToolChoice) *toolConfig {
	switch choice.Mode {
	case models.ToolChoiceNone:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "NONE"}}
	case models.ToolChoiceRequired:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "ANY"}}
	case models.ToolChoiceTool:
		if choice.Tool != "" {
			return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "ANY", AllowedFunctionNames: []string{choice.Tool}}}
		}
	}
	return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "AUTO"}}
}

func toContents(msgs []models.Message) []content {
	var out []content
	for _, m := range msgs {
		c := content{Role: "user"}
		if m.Role == models.RoleAssistant {
			c.Role = "model"
		}
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				if v.Text != "" {
					c.Parts = append(c.Parts, part{Text: v.Text})
				}
			case models.ImagePart:
				c.Parts = append(c.Parts, part{InlineData: &inlineData{MimeType: v.MimeType, Data: v.Data}})
			case models.ReasoningPart:
				c.Parts = append(c.Parts, part{Text: v.Reasoning, Thought: true})
			case models.ToolCallPart:
				c.Parts = append(c.Parts, part{FunctionCall: &functionCall{Name: v.Name, Args: v.Input}})
			case models.ToolResultPart:
				c.Parts = append(c.Parts, part{FunctionResponse: &functionResponse{Name: v.CallID, Response: map[string]any{"name": v.CallID, "content": shared.ToolResultText(v.Output)}}})
			}
		}
		if len(c.Parts) > 0 {
			out = append(out, c)
		}
	}
	return out
}

type parser struct {
	out          *models.EventStream[models.StreamPart, *models.Message]
	origin       *models.MessageOrigin
	content      models.Content
	textStarted  bool
	textBuf      strings.Builder
	reasoningBuf strings.Builder
	usage        models.Usage
	finishReason string
	nextToolID   int
	hasToolCalls bool
	terminated   bool
}

func newParser(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin) *parser {
	return &parser{out: out, origin: origin}
}

func (p *parser) handle(data string) bool {
	var ev struct {
		Candidates []struct {
			Content      content `json:"content"`
			FinishReason string  `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
			TotalTokenCount         int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		p.terminate(models.FinishReasonError, err)
		return true
	}
	if ev.UsageMetadata.TotalTokenCount > 0 {
		p.usage = models.Usage{InputTokens: ev.UsageMetadata.PromptTokenCount - ev.UsageMetadata.CachedContentTokenCount, OutputTokens: ev.UsageMetadata.CandidatesTokenCount + ev.UsageMetadata.ThoughtsTokenCount, CachedInputTokens: ev.UsageMetadata.CachedContentTokenCount, ReasoningTokens: ev.UsageMetadata.ThoughtsTokenCount, TotalTokens: ev.UsageMetadata.TotalTokenCount}
	}
	if len(ev.Candidates) == 0 {
		return false
	}
	c := ev.Candidates[0]
	if c.FinishReason != "" {
		p.finishReason = c.FinishReason
	}
	for _, part := range c.Content.Parts {
		if part.Text != "" && !part.Thought {
			if !p.textStarted {
				p.textStarted = true
				p.out.Push(models.TextStartPart{ID: "text_0"})
			}
			p.textBuf.WriteString(part.Text)
			p.out.Push(models.TextDeltaPart{ID: "text_0", Delta: part.Text})
		}
		if part.Text != "" && part.Thought {
			if p.reasoningBuf.Len() == 0 {
				p.out.Push(models.ReasoningStartPart{ID: "reasoning_0"})
			}
			p.reasoningBuf.WriteString(part.Text)
			p.out.Push(models.ReasoningDeltaPart{ID: "reasoning_0", Delta: part.Text})
		}
		if part.FunctionCall != nil {
			id := "tool_" + strconv.Itoa(p.nextToolID)
			p.nextToolID++
			p.hasToolCalls = true
			p.content = append(p.content, models.ToolCallPart{CallID: id, Name: part.FunctionCall.Name, Input: part.FunctionCall.Args})
			p.out.Push(models.ToolInputStartPart{ID: id, ToolName: part.FunctionCall.Name})
			p.out.Push(models.ToolInputEndPart{ID: id})
			p.out.Push(models.ToolCallPart_{ID: id, ToolName: part.FunctionCall.Name, Input: part.FunctionCall.Args})
		}
	}
	return false
}

func (p *parser) finish() { p.terminate(mapFinishReason(p.finishReason, p.hasToolCalls), nil) }

func mapFinishReason(reason string, hasToolCalls bool) models.FinishReason {
	if reason == "STOP" && hasToolCalls {
		return models.FinishReasonToolCalls
	}
	switch reason {
	case "", "STOP":
		return models.FinishReasonStop
	case "MAX_TOKENS":
		return models.FinishReasonLength
	case "SAFETY", "RECITATION", "IMAGE_SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return models.FinishReasonContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return models.FinishReasonError
	default:
		return models.FinishReasonOther
	}
}

func (p *parser) terminate(reason models.FinishReason, err error) {
	if p.terminated {
		return
	}
	p.terminated = true
	if p.reasoningBuf.Len() > 0 {
		p.content = append(p.content, models.ReasoningPart{Reasoning: p.reasoningBuf.String()})
		p.out.Push(models.ReasoningEndPart{ID: "reasoning_0"})
	}
	if p.textStarted {
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
