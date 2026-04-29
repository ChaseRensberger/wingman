// Package anthropic implements wingmodels.Model for Anthropic's Messages API.
//
// Wire reference: https://docs.anthropic.com/en/api/messages-streaming
//
// Stream mapping (Anthropic SSE -> wingmodels.StreamPart):
//
//	message_start          -> StreamStartPart{} + ResponseMetadataPart{id, model}
//	content_block_start
//	  text                 -> TextStartPart{id=block_index}
//	  thinking             -> ReasoningStartPart{id=block_index}
//	  tool_use             -> ToolInputStartPart{id=tool_use_id, tool_name}
//	content_block_delta
//	  text_delta           -> TextDeltaPart{id, delta}
//	  thinking_delta       -> ReasoningDeltaPart{id, delta}
//	  input_json_delta     -> ToolInputDeltaPart{id=tool_use_id, delta}
//	content_block_stop
//	  text                 -> TextEndPart{id}
//	  thinking             -> ReasoningEndPart{id}
//	  tool_use             -> ToolInputEndPart{id} + ToolCallPart_{id, name, input}
//	message_delta          -> (accumulated; emitted via FinishPart)
//	message_stop           -> FinishPart{reason, usage, message}
//	error                  -> ErrorPart + FinishPart{reason: error}
//	ping                   -> (skipped)
//
// Anthropic stop reasons map to wingmodels.FinishReason:
//
//	end_turn      -> stop
//	max_tokens    -> length
//	stop_sequence -> stop
//	tool_use      -> tool-calls
//	pause_turn    -> stop  (treated as end-of-turn for now)
//	refusal       -> content-filter
//	(empty/other) -> unknown
package anthropic

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

// Meta is the registry entry for the Anthropic provider.
//
// Factory returns a *Client typed as wingmodels.Model. Callers that need
// provider-specific knobs construct *Client directly via New.
var Meta = provider.ProviderMeta{
	ID:        "anthropic",
	Name:      "Anthropic",
	AuthTypes: []provider.AuthType{provider.AuthTypeAPIKey},
	Factory: func(opts map[string]any) (wingmodels.Model, error) {
		return New(Config{Options: opts})
	},
}

func init() { provider.Register(Meta) }

// Config controls construction of a Client. Options is the open-ended bag the
// registry passes through; explicit fields take precedence.
type Config struct {
	APIKey     string
	Model      string
	MaxTokens  int
	MaxRetries int
	Options    map[string]any
}

const (
	defaultModel      = "claude-haiku-4-5"
	defaultMaxTokens  = 4096
	defaultMaxRetries = 3
	apiURL            = "https://api.anthropic.com/v1/messages"
	apiTokenURL       = "https://api.anthropic.com/v1/messages/count_tokens"
	apiVersion        = "2023-06-01"
	httpTimeout       = 5 * time.Minute
	maxRetryDelay     = 60 * time.Second
)

// Client is a configured Anthropic Model. One Client = one model id; create
// separate Clients for separate models.
type Client struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
	maxRetries int
}

// New constructs a Client. API key resolution order: Config.APIKey, then
// Options["api_key"], then ANTHROPIC_API_KEY env var. Returns an error if
// none are set.
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
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: no API key (set Config.APIKey, Options[\"api_key\"], or ANTHROPIC_API_KEY)")
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
		maxTokens:  maxTokens,
		httpClient: &http.Client{Timeout: httpTimeout},
		maxRetries: maxRetries,
	}, nil
}

// Info returns the catalog ModelInfo for this client's model. If the model is
// not in the catalog (e.g. a brand-new id), returns a minimal ModelInfo with
// just provider+id populated. The API field is always stamped to
// APIAnthropicMessages so the transform layer can detect same-model replays.
func (c *Client) Info() wingmodels.ModelInfo {
	if info, ok := catalog.Get("anthropic", c.model); ok {
		info.API = wingmodels.APIAnthropicMessages
		return info
	}
	return wingmodels.ModelInfo{
		Provider: "anthropic",
		ID:       c.model,
		API:      wingmodels.APIAnthropicMessages,
	}
}

// origin returns the MessageOrigin stamped on every assistant message this
// client produces. Used by the transform layer at the next request to detect
// same-model replay and skip lossy normalizations (reasoning drop, tool-call
// id rename).
func (c *Client) origin() *wingmodels.MessageOrigin {
	return &wingmodels.MessageOrigin{
		Provider: "anthropic",
		API:      wingmodels.APIAnthropicMessages,
		ModelID:  c.model,
	}
}

// CountTokens calls Anthropic's /v1/messages/count_tokens for an exact count
// of input tokens. Network round-trip; no fallback.
func (c *Client) CountTokens(ctx context.Context, msgs []wingmodels.Message) (int, error) {
	body, err := json.Marshal(struct {
		Model    string             `json:"model"`
		Messages []anthropicMessage `json:"messages"`
	}{Model: c.model, Messages: c.toWireMessages(msgs)})
	if err != nil {
		return 0, fmt.Errorf("anthropic: marshal count_tokens: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiTokenURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("anthropic: build count_tokens request: %w", err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("anthropic: count_tokens request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("anthropic: read count_tokens: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("anthropic: count_tokens %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, fmt.Errorf("anthropic: parse count_tokens: %w", err)
	}
	return out.InputTokens, nil
}

// Stream begins a streaming Messages request. Setup failures (marshal,
// network refused) return (nil, error); after the goroutine starts, all
// failures terminate via ErrorPart + FinishPart{reason: error|aborted}.
//
// Before serializing, req.Messages is normalized via transform.Apply for
// this target (drops failed/aborted assistant turns, drops cross-model
// reasoning blocks, downgrades images when the model lacks vision,
// reconciles orphaned tool calls, elides empty messages). The transform is
// pure; the caller's slice is not mutated.
func (c *Client) Stream(ctx context.Context, req wingmodels.Request) (*wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], error) {
	info := c.Info()
	req.Messages = transform.Apply(req.Messages, transform.Target{
		Provider:     "anthropic",
		API:          wingmodels.APIAnthropicMessages,
		ModelID:      c.model,
		Capabilities: info.Capabilities,
	})

	built := c.buildRequest(req)
	built.wire.Stream = true

	body, err := json.Marshal(built.wire)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	resp, err := c.doStreamWithRetry(ctx, body, built.needsThinkingBeta)
	if err != nil {
		return nil, err
	}

	// 64-event buffer is generous; Anthropic typically emits a few events
	// per second and the consumer drains synchronously in the agent loop.
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](64)
	go runStream(ctx, resp, out, c.origin())
	return out, nil
}

// runStream owns the http.Response and emits parsed events on out. It MUST
// close out exactly once before returning, with either the assembled message
// or a terminal error. origin is stamped on the assembled message so the
// next request's transform pass can detect same-model replay.
func runStream(ctx context.Context, resp *http.Response, out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], origin *wingmodels.MessageOrigin) {
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Anthropic events can carry large tool-input chunks; bump the line
	// buffer well past Anthropic's max single-event size.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	p := newStreamParser(out, origin)

	var eventType string
	for scanner.Scan() {
		// Honor cancellation on every line. Without this the SSE read can
		// block past ctx cancellation until Anthropic next emits.
		if err := ctx.Err(); err != nil {
			p.terminateAborted()
			return
		}

		line := scanner.Text()
		switch {
		case line == "":
			// SSE event boundary; nothing to emit.
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			if done := p.handle(eventType, data); done {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		p.terminateError(fmt.Errorf("anthropic: scanner: %w", err))
		return
	}
	// Stream ended without a terminator. Treat as error so consumers don't
	// hang waiting for FinishPart.
	p.terminateError(fmt.Errorf("anthropic: stream closed without message_stop"))
}

// streamParser holds the in-flight assembly state for one streaming response.
// Anthropic streams content blocks by index; we map each index to its
// wingmodels block id (we use a stable string derived from the index since
// Anthropic doesn't supply ids for text/thinking blocks). For tool_use blocks
// the Anthropic-supplied tool_use_id is the wingmodels id.
type streamParser struct {
	out      *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]
	terminated bool

	// origin is stamped on the assembled assistant message so downstream
	// transform.Apply calls can detect same-model replay. Constant for the
	// life of one stream.
	origin *wingmodels.MessageOrigin

	// Per-block bookkeeping keyed by Anthropic content block index.
	blocks map[int]*blockState

	// Final assembly.
	content      wingmodels.Content
	usage        wingmodels.Usage
	stopReason   string
	responseMeta wingmodels.ResponseMetadata
	startEmitted bool
}

type blockKind int

const (
	blockText blockKind = iota
	blockReasoning
	blockToolUse
)

type blockState struct {
	kind     blockKind
	id       string         // wingmodels block id (also tool_use_id for tool blocks)
	toolName string         // tool_use only
	textBuf  strings.Builder
	jsonBuf  strings.Builder // raw input JSON for tool_use
	// Reasoning extras: Anthropic may emit a signature_delta (extended
	// thinking redacted-payload) we must round-trip on next turn.
	signature strings.Builder
	redacted  bool
}

func newStreamParser(out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message], origin *wingmodels.MessageOrigin) *streamParser {
	return &streamParser{out: out, blocks: make(map[int]*blockState), origin: origin}
}

// handle dispatches one parsed SSE event. Returns true when the stream is
// terminated (no more events should be processed).
func (p *streamParser) handle(eventType, data string) bool {
	switch eventType {
	case "message_start":
		var ev struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("anthropic: parse message_start: %w", err))
			return true
		}
		p.responseMeta = wingmodels.ResponseMetadata{ID: ev.Message.ID, ModelID: ev.Message.Model}
		p.usage.InputTokens = ev.Message.Usage.InputTokens
		p.usage.OutputTokens = ev.Message.Usage.OutputTokens
		// Emit stream-start (no warnings; Anthropic doesn't surface them
		// in this shape) followed by response-metadata.
		p.out.Push(wingmodels.StreamStartPart{})
		p.startEmitted = true
		p.out.Push(wingmodels.ResponseMetadataPart{ResponseMetadata: p.responseMeta})

	case "content_block_start":
		var ev struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id,omitempty"`
				Name string `json:"name,omitempty"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("anthropic: parse content_block_start: %w", err))
			return true
		}

		switch ev.ContentBlock.Type {
		case "text":
			id := blockID(ev.Index)
			p.blocks[ev.Index] = &blockState{kind: blockText, id: id}
			p.out.Push(wingmodels.TextStartPart{ID: id})
		case "thinking":
			id := blockID(ev.Index)
			p.blocks[ev.Index] = &blockState{kind: blockReasoning, id: id}
			p.out.Push(wingmodels.ReasoningStartPart{ID: id})
		case "redacted_thinking":
			// Anthropic sends only the encrypted payload; treat as a
			// reasoning block that's redacted. The signature comes via
			// the content_block itself, not a delta.
			id := blockID(ev.Index)
			b := &blockState{kind: blockReasoning, id: id, redacted: true}
			p.blocks[ev.Index] = b
			p.out.Push(wingmodels.ReasoningStartPart{ID: id})
		case "tool_use":
			id := ev.ContentBlock.ID
			p.blocks[ev.Index] = &blockState{kind: blockToolUse, id: id, toolName: ev.ContentBlock.Name}
			p.out.Push(wingmodels.ToolInputStartPart{ID: id, ToolName: ev.ContentBlock.Name})
		default:
			// Unknown block kind: skip. We'll log if/when we add a logger.
		}

	case "content_block_delta":
		var ev struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text,omitempty"`
				Thinking    string `json:"thinking,omitempty"`
				Signature   string `json:"signature,omitempty"`
				PartialJSON string `json:"partial_json,omitempty"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("anthropic: parse content_block_delta: %w", err))
			return true
		}
		b := p.blocks[ev.Index]
		if b == nil {
			return false // unknown block; ignore
		}
		switch ev.Delta.Type {
		case "text_delta":
			b.textBuf.WriteString(ev.Delta.Text)
			p.out.Push(wingmodels.TextDeltaPart{ID: b.id, Delta: ev.Delta.Text})
		case "thinking_delta":
			b.textBuf.WriteString(ev.Delta.Thinking)
			p.out.Push(wingmodels.ReasoningDeltaPart{ID: b.id, Delta: ev.Delta.Thinking})
		case "signature_delta":
			// Signature is opaque encrypted material for redacted thinking
			// or tool-use replay. Accumulate; emit only at content_block_stop.
			b.signature.WriteString(ev.Delta.Signature)
		case "input_json_delta":
			b.jsonBuf.WriteString(ev.Delta.PartialJSON)
			p.out.Push(wingmodels.ToolInputDeltaPart{ID: b.id, Delta: ev.Delta.PartialJSON})
		}

	case "content_block_stop":
		var ev struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		b := p.blocks[ev.Index]
		if b == nil {
			return false
		}
		switch b.kind {
		case blockText:
			p.content = append(p.content, wingmodels.TextPart{Text: b.textBuf.String()})
			p.out.Push(wingmodels.TextEndPart{ID: b.id})
		case blockReasoning:
			p.content = append(p.content, wingmodels.ReasoningPart{
				Reasoning: b.textBuf.String(),
				Signature: b.signature.String(),
				Redacted:  b.redacted,
			})
			p.out.Push(wingmodels.ReasoningEndPart{ID: b.id})
		case blockToolUse:
			var input map[string]any
			if b.jsonBuf.Len() > 0 {
				if err := json.Unmarshal([]byte(b.jsonBuf.String()), &input); err != nil {
					// Surface the parse failure but don't terminate; the
					// model may have produced malformed args and the agent
					// loop should be allowed to handle/retry.
					p.out.Push(wingmodels.ErrorPart{
						Message: fmt.Sprintf("anthropic: tool input json: %v", err),
						Code:    "invalid_tool_input",
					})
				}
			}
			p.content = append(p.content, wingmodels.ToolCallPart{
				CallID: b.id,
				Name:   b.toolName,
				Input:  input,
			})
			p.out.Push(wingmodels.ToolInputEndPart{ID: b.id})
			p.out.Push(wingmodels.ToolCallPart_{ID: b.id, ToolName: b.toolName, Input: input})
		}
		delete(p.blocks, ev.Index)

	case "message_delta":
		var ev struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			p.terminateError(fmt.Errorf("anthropic: parse message_delta: %w", err))
			return true
		}
		p.stopReason = ev.Delta.StopReason
		// Anthropic re-reports cumulative output_tokens here.
		p.usage.OutputTokens = ev.Usage.OutputTokens

	case "message_stop":
		p.terminateNormal()
		return true

	case "error":
		var ev struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		p.out.Push(wingmodels.ErrorPart{Message: ev.Error.Message, Code: ev.Error.Type})
		p.terminate(wingmodels.FinishReasonError, fmt.Errorf("anthropic: %s: %s", ev.Error.Type, ev.Error.Message))
		return true

	case "ping":
		// Keep-alive; ignore.
	}
	return false
}

func (p *streamParser) terminateNormal() {
	p.usage.TotalTokens = p.usage.InputTokens + p.usage.OutputTokens
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
		// Treat as setup failure path: emit minimal stream-start so consumers
		// have a consistent prefix before the error.
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

// blockID returns the wingmodels stream-part id for an Anthropic content
// block at the given index. Stable per-stream; "blk_<index>" is opaque to
// consumers who only correlate within one stream.
func blockID(index int) string { return "blk_" + strconv.Itoa(index) }

// mapStopReason converts Anthropic's stop_reason into our normalized enum.
func mapStopReason(r string) wingmodels.FinishReason {
	switch r {
	case "end_turn", "stop_sequence", "pause_turn":
		return wingmodels.FinishReasonStop
	case "max_tokens":
		return wingmodels.FinishReasonLength
	case "tool_use":
		return wingmodels.FinishReasonToolCalls
	case "refusal":
		return wingmodels.FinishReasonContentFilter
	case "":
		return wingmodels.FinishReasonUnknown
	default:
		return wingmodels.FinishReasonOther
	}
}

// ---- request building ------------------------------------------------------

// anthropicMessage and contentBlock are the wire shapes we send to Anthropic.
// They are NOT the wingmodels Message/Part shapes; they are the Anthropic
// API's format. Conversion happens in toWireMessages.
type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`         // tool_use
	Name      string         `json:"name,omitempty"`       // tool_use
	Input     map[string]any `json:"input,omitempty"`      // tool_use
	ToolUseID string         `json:"tool_use_id,omitempty"` // tool_result
	Content   any            `json:"content,omitempty"`    // tool_result (string or []contentBlock)
	IsError   bool           `json:"is_error,omitempty"`   // tool_result
	// Extended thinking round-trip fields:
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"` // redacted_thinking encrypted payload
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type request struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	Tools        []toolDefinition   `json:"tools,omitempty"`
	ToolChoice   *toolChoice        `json:"tool_choice,omitempty"`
	Thinking     *thinkingConfig    `json:"thinking,omitempty"`
	Stream       bool               `json:"stream,omitempty"`
	OutputConfig *outputConfig      `json:"output_config,omitempty"`
}

// outputConfig carries Anthropic's structured-output request configuration.
// Maps onto output_config.format = {type: "json_schema", schema: {...}}.
type outputConfig struct {
	Format *outputFormat `json:"format,omitempty"`
}

type outputFormat struct {
	Type   string         `json:"type"` // "json_schema"
	Schema map[string]any `json:"schema"`
}

// toolChoice maps to Anthropic's tool_choice object.
// type is one of "auto", "any", "none", "tool".
// name is required when type == "tool".
type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// thinkingConfig enables extended thinking on the request.
// For budget-based models: {"type":"enabled","budget_tokens":N}
// For adaptive models:     {"type":"adaptive","display":"summarized"}
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

// builtRequest is the internal result of buildRequest. It bundles the wire
// struct with any extra request-level metadata (e.g. beta headers) that Stream
// needs to inject before sending.
type builtRequest struct {
	wire             request
	needsThinkingBeta bool // inject interleaved-thinking beta header
}

// supportsAdaptiveThinking returns true for Anthropic models that use the
// new adaptive thinking API (claude-opus-4+, claude-sonnet-4-5 / sonnet-4-6).
// Budget-based models use the older {"type":"enabled","budget_tokens":N} form.
func supportsAdaptiveThinking(modelID string) bool {
	adaptive := []string{
		"claude-opus-4",
		"claude-sonnet-4-5",
		"claude-sonnet-4-6",
	}
	for _, prefix := range adaptive {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func (c *Client) buildRequest(req wingmodels.Request) builtRequest {
	maxOut := req.MaxOutputTokens
	if maxOut == 0 {
		maxOut = c.maxTokens
	}
	r := request{
		Model:     c.model,
		MaxTokens: maxOut,
		System:    req.System,
		Messages:  c.toWireMessages(req.Messages),
	}

	// Tools
	if len(req.Tools) > 0 {
		r.Tools = make([]toolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i] = toolDefinition{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
		}
	}

	// ToolChoice
	// NOTE: disable_parallel_tool_use is not supported by the wingmodels
	// abstraction; we omit it and let Anthropic default to allowing parallel
	// tool calls.
	if len(r.Tools) > 0 {
		switch req.ToolChoice.Mode {
		case wingmodels.ToolChoiceRequired:
			r.ToolChoice = &toolChoice{Type: "any"}
		case wingmodels.ToolChoiceNone:
			r.ToolChoice = &toolChoice{Type: "none"}
		case wingmodels.ToolChoiceTool:
			if req.ToolChoice.Tool != "" {
				r.ToolChoice = &toolChoice{Type: "tool", Name: req.ToolChoice.Tool}
			}
		// ToolChoiceAuto and zero-value: omit — Anthropic defaults to auto.
		}
	}

	// Thinking (extended thinking / reasoning)
	var needsBeta bool
	if th := req.Capabilities.Thinking; th != nil {
		if supportsAdaptiveThinking(c.model) {
			// Adaptive API (claude-4+ era): effort-based or default.
			display := "summarized"
			r.Thinking = &thinkingConfig{Type: "adaptive", Display: display}
			// output_config.effort not yet in this struct; add if needed.
		} else {
			// Budget-based API (claude-3.x era).
			budget := th.BudgetTokens
			if budget <= 0 {
				budget = 1024
			}
			r.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: budget}
			// Interleaved thinking requires the beta header for non-adaptive models.
			needsBeta = true
			// Budget-based thinking requires max_tokens > budget.
			if r.MaxTokens <= budget {
				r.MaxTokens = budget + 1024
			}
		}
	}

	// Structured output (output_config.format = json_schema)
	if req.OutputSchema != nil {
		r.OutputConfig = &outputConfig{
			Format: &outputFormat{
				Type:   "json_schema",
				Schema: req.OutputSchema.Schema,
			},
		}
	}

	return builtRequest{wire: r, needsThinkingBeta: needsBeta}
}

// toWireMessages converts wingmodels.Message to Anthropic's wire format.
//
// Role mapping:
//   - user, assistant -> same
//   - tool -> "user" with tool_result content blocks (Anthropic places tool
//     results in the user turn, not a separate role)
//
// Part mapping:
//   - TextPart       -> {type:"text", text}
//   - ReasoningPart  -> {type:"thinking", thinking, signature} or
//                       {type:"redacted_thinking", data: signature} when redacted
//   - ToolCallPart   -> {type:"tool_use", id, name, input}
//   - ToolResultPart -> {type:"tool_result", tool_use_id, content, is_error}
//   - ImagePart      -> currently dropped (image input not wired in v0.1)
func (c *Client) toWireMessages(msgs []wingmodels.Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		role := string(m.Role)
		if m.Role == wingmodels.RoleTool {
			role = "user"
		}
		blocks := make([]contentBlock, 0, len(m.Content))
		for _, p := range m.Content {
			switch v := p.(type) {
			case wingmodels.TextPart:
				if strings.TrimSpace(v.Text) == "" {
					continue
				}
				blocks = append(blocks, contentBlock{Type: "text", Text: v.Text})
			case wingmodels.ReasoningPart:
				if v.Redacted {
					blocks = append(blocks, contentBlock{Type: "redacted_thinking", Data: v.Signature})
				} else {
					blocks = append(blocks, contentBlock{Type: "thinking", Thinking: v.Reasoning, Signature: v.Signature})
				}
			case wingmodels.ToolCallPart:
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: v.CallID, Name: v.Name, Input: v.Input})
			case wingmodels.ToolResultPart:
				blocks = append(blocks, contentBlock{
					Type:      "tool_result",
					ToolUseID: v.CallID,
					Content:   toolResultContent(v.Output),
					IsError:   v.IsError,
				})
			case wingmodels.ImagePart:
				// Skipped in v0.1; image input requires Anthropic's image
				// content block which we'll wire when we surface ImagePart
				// in the agent loop.
			}
		}
		if len(blocks) == 0 {
			continue
		}
		out = append(out, anthropicMessage{Role: role, Content: blocks})
	}
	return out
}

// toolResultContent flattens a Part slice to Anthropic's tool_result content
// shape. Anthropic accepts either a plain string (single text part) or an
// array of content blocks (text + image). We pick the simplest representation
// for the input.
func toolResultContent(out []wingmodels.Part) any {
	if len(out) == 0 {
		return ""
	}
	// Fast path: single text part -> string.
	if len(out) == 1 {
		if t, ok := out[0].(wingmodels.TextPart); ok {
			return t.Text
		}
	}
	blocks := make([]contentBlock, 0, len(out))
	for _, p := range out {
		switch v := p.(type) {
		case wingmodels.TextPart:
			blocks = append(blocks, contentBlock{Type: "text", Text: v.Text})
		case wingmodels.ImagePart:
			// Anthropic image source format:
			// {type:"image", source:{type:"base64", media_type, data}}.
			// Encode inline since we don't reuse contentBlock for it.
			blocks = append(blocks, contentBlock{
				Type: "image",
				// Stuff the source object into Content; the "content" field
				// is `any` so it round-trips. This is ugly; the cleaner fix
				// is a dedicated image block type, deferred to when we wire
				// image input end-to-end.
				Content: map[string]any{
					"type":       "base64",
					"media_type": v.MimeType,
					"data":       v.Data,
				},
			})
		}
	}
	return blocks
}

// ---- HTTP plumbing ---------------------------------------------------------

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
}

// betaInterleavedThinking is the Anthropic beta header value required when
// extended thinking is enabled on non-adaptive (budget-based) models.
const betaInterleavedThinking = "interleaved-thinking-2025-05-14"

func (c *Client) doStreamWithRetry(ctx context.Context, body []byte, thinkingBeta bool) (*http.Response, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.doRequest(ctx, body, thinkingBeta)
		if err != nil {
			if shouldRetryTransport(err) && attempt < c.maxRetries {
				if waitErr := waitForRetry(ctx, backoffDelay(attempt)); waitErr != nil {
					return nil, fmt.Errorf("anthropic: %w", waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("anthropic: request: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if shouldRetryStatus(resp.StatusCode) && attempt < c.maxRetries {
			if waitErr := waitForRetry(ctx, retryDelay(resp, attempt)); waitErr != nil {
				return nil, fmt.Errorf("anthropic: API %d: %s", resp.StatusCode, string(raw))
			}
			continue
		}
		return nil, fmt.Errorf("anthropic: API %d: %s", resp.StatusCode, string(raw))
	}
	return nil, fmt.Errorf("anthropic: retries exhausted")
}

func (c *Client) doRequest(ctx context.Context, body []byte, thinkingBeta bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	if thinkingBeta {
		req.Header.Set("anthropic-beta", betaInterleavedThinking)
	}
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

// Compile-time check that *Client satisfies wingmodels.Model.
var _ wingmodels.Model = (*Client)(nil)
