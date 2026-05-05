// Package compaction is the canonical wingagent plugin: it summarizes
// the head of a long message history into a single inline marker so
// long-running sessions stay under the model's context window without
// losing ground-truth on disk.
//
// # Design
//
// Compaction has two halves:
//
//   - Write-side (BeforeStep): when input tokens approach the model's
//     context window, summarize every message between the previous
//     marker (if any) and the keep-tail boundary, then *append* a
//     MarkerPart message into the running history. The original
//     messages are NOT removed — they remain in the durable store and
//     in the loop's running history. This is opencode's model: the
//     transcript is append-only; markers act as bookmarks the read-side
//     uses to elide stale context.
//
//   - Read-side (TransformContext): walk the per-turn message slice;
//     find the latest MarkerPart; build the model-facing view as
//     [synthesized summary text] + [messages after the marker]. The
//     model never sees the original pre-marker messages. The session
//     history is unaffected — only the wire request is.
//
// # Why two seams
//
// Single-seam approaches (truncate-and-replace in BeforeStep) lose
// history irrecoverably and prevent UIs from showing what was
// compacted. Splitting write (append marker) from read (filter) keeps
// every byte addressable and lets observability surfaces render the
// pre-compaction transcript verbatim.
//
// # Token estimation
//
// The hook calls Model.CountTokens against the *current* message
// snapshot, which fixes two bugs in any "estimate from last turn's
// usage" approach: first-call blindness (no prior turn) and lag-by-one
// (the call that overflows happens before its usage is reported).
// CountTokens errors fall back to a chars/4 heuristic so a flaky
// counter endpoint never blocks the loop.
//
// # Usage
//
//	sess := session.New(
//	    session.WithModel(m),
//	    session.WithPlugin(compaction.New()),
//	)
//
// To customize:
//
//	session.WithPlugin(compaction.New(
//	    compaction.WithThreshold(0.7),
//	    compaction.WithKeepTail(8),
//	))
package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/plugin"
	"github.com/chaserensberger/wingman/wingmodels"
)

// PartType is the discriminator string MarkerPart serializes with.
// Stable; persisted in storage and on the SSE wire. The session stream
// classifier inspects this string to surface the dedicated "compaction"
// SSE event type without importing this package.
const PartType = "compaction_marker"

// MarkerPart records that a span of conversation history was
// summarized. Inserted by the plugin's BeforeStep hook in append
// position; the read-side TransformContext hook turns it into a
// TextPart for the model and drops everything before it.
type MarkerPart struct {
	// Summary is the natural-language summary of the messages this
	// marker replaces in the model-facing view.
	Summary string `json:"summary"`
	// OriginalCount is how many messages were summarized. Useful for
	// UI labels ("Compacted 12 messages") and debugging.
	OriginalCount int `json:"original_count"`
	// CompactedAt is the RFC3339 UTC timestamp when compaction ran.
	CompactedAt string `json:"compacted_at"`
}

func (MarkerPart) Type() string { return PartType }

// MarshalJSON / UnmarshalJSON: defaults via field tags are sufficient.
// The wingmodels.Part interface's unexported isPart marker means
// MarkerPart cannot satisfy Part by name from outside wingmodels. We
// route through wingmodels.OpaquePart at the registry seam: the part
// type is registered with a decoder that returns a *typed* part wrapped
// in an adapter, but the adapter still needs to satisfy Part.
//
// Workaround: the Plugin's RegisterPart decoder returns a
// wingmodels.OpaquePart whose Raw is the marker's JSON. Read-side
// callers that want typed access call DecodeMarker(part) which extracts
// MarkerPart from the OpaquePart's bytes. This keeps wingmodels' Part
// union sealed (no external types satisfy it directly) while letting
// plugins ship "logical" Part types over the OpaquePart carrier.

// DecodeMarker extracts a MarkerPart from a wingmodels.Part if it
// represents a compaction marker. Returns ok=false for any other
// part. Safe to call on every part during a content walk.
func DecodeMarker(p wingmodels.Part) (MarkerPart, bool) {
	if p == nil || p.Type() != PartType {
		return MarkerPart{}, false
	}
	op, ok := p.(wingmodels.OpaquePart)
	if !ok {
		return MarkerPart{}, false
	}
	var m MarkerPart
	if err := json.Unmarshal(op.Raw, &m); err != nil {
		return MarkerPart{}, false
	}
	return m, true
}

// newMarkerPart constructs a wingmodels.Part carrying a MarkerPart's
// payload. Implemented as an OpaquePart so it satisfies wingmodels.Part
// without breaking the sealed-union invariant.
func newMarkerPart(m MarkerPart) (wingmodels.Part, error) {
	body, err := json.Marshal(struct {
		Type          string `json:"type"`
		Summary       string `json:"summary"`
		OriginalCount int    `json:"original_count"`
		CompactedAt   string `json:"compacted_at"`
	}{PartType, m.Summary, m.OriginalCount, m.CompactedAt})
	if err != nil {
		return nil, err
	}
	return wingmodels.OpaquePart{TypeName: PartType, Raw: body}, nil
}

// Option configures a Plugin.
type Option func(*Plugin)

// Plugin is the compaction plugin instance.
type Plugin struct {
	threshold     float64
	keepTail      int
	minMessages   int
	summaryPrompt string
	model         wingmodels.Model
}

// New constructs a compaction plugin with the supplied options applied
// over the defaults: threshold 0.85, keepTail 4, minMessages 6.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		threshold:   0.85,
		keepTail:    4,
		minMessages: 6,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// WithThreshold sets the input-tokens / context-window ratio that
// triggers compaction. Default 0.85.
func WithThreshold(f float64) Option { return func(p *Plugin) { p.threshold = f } }

// WithKeepTail sets how many trailing messages survive compaction
// untouched. Default 4. (These are the messages between the new marker
// and the end of history.)
func WithKeepTail(n int) Option { return func(p *Plugin) { p.keepTail = n } }

// WithMinMessages sets the floor below which compaction never runs.
// Default 6. Setting this to 0 disables the floor.
func WithMinMessages(n int) Option { return func(p *Plugin) { p.minMessages = n } }

// WithSummaryPrompt overrides the summarization system prompt.
func WithSummaryPrompt(s string) Option { return func(p *Plugin) { p.summaryPrompt = s } }

// WithModel uses a specific model for the summarization sub-call.
// Default: use the loop's model at invocation time. Useful when
// summarization should run on a cheaper / faster / longer-context
// model than the main conversation.
func WithModel(m wingmodels.Model) Option { return func(p *Plugin) { p.model = m } }

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return "compaction" }

// Install implements plugin.Plugin. Registers the marker Part decoder,
// the BeforeStep write-side hook, and the TransformContext read-side
// filter.
func (p *Plugin) Install(r *plugin.Registry) error {
	// Part decoder: return an OpaquePart preserving the bytes. The
	// payload is small and DecodeMarker re-parses on demand; storing
	// raw bytes avoids needing a wingmodels.Part-satisfying typed
	// wrapper (the Part union is sealed to wingmodels).
	r.RegisterPart(PartType, func(data []byte) (wingmodels.Part, error) {
		raw := make([]byte, len(data))
		copy(raw, data)
		return wingmodels.OpaquePart{TypeName: PartType, Raw: raw}, nil
	})

	r.RegisterBeforeStep(p.beforeStep)
	r.RegisterTransformContext(p.transformContext)
	return nil
}

// beforeStep is the write-side seam. When token usage crosses the
// threshold, summarize every message after the most recent marker (or
// from the start, if none) up to the keepTail boundary, then append a
// new marker. The pre-compaction messages are kept in history so the
// transcript remains addressable on disk and via History().
func (p *Plugin) beforeStep(ctx context.Context, info loop.BeforeStepInfo) ([]wingmodels.Message, error) {
	model := p.model
	if model == nil {
		model = info.Model
	}
	if model == nil {
		return info.Messages, nil
	}

	if len(info.Messages) < p.minMessages {
		return info.Messages, nil
	}
	ctxWindow := model.Info().ContextWindow
	if ctxWindow <= 0 {
		return info.Messages, nil
	}

	// Token estimate against the current snapshot. CountTokens is
	// part of the Model contract; on error we fall back to chars/4.
	tokens, err := model.CountTokens(ctx, info.Messages)
	if err != nil {
		tokens = approxTokens(info.Messages)
	}
	if float64(tokens)/float64(ctxWindow) < p.threshold {
		return info.Messages, nil
	}

	// Find the latest marker so we don't re-summarize what's already
	// summarized. We summarize messages in [latestMarkerIdx+1, tailStart).
	latestMarkerIdx := findLatestMarker(info.Messages)
	tailStart := len(info.Messages) - p.keepTail
	if tailStart <= latestMarkerIdx+1 {
		// Nothing new to summarize since the last marker.
		return info.Messages, nil
	}

	headStart := latestMarkerIdx + 1
	head := info.Messages[headStart:tailStart]
	if len(head) == 0 {
		return info.Messages, nil
	}

	summary, err := summarize(ctx, model, p.summaryPrompt, head)
	if err != nil {
		return nil, fmt.Errorf("summarize: %w", err)
	}

	markerPart, err := newMarkerPart(MarkerPart{
		Summary:       summary,
		OriginalCount: len(head),
		CompactedAt:   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("build marker: %w", err)
	}

	markerMsg := wingmodels.Message{
		Role:    wingmodels.RoleUser,
		Content: wingmodels.Content{markerPart},
	}

	// Append marker between head and tail. Result:
	//   [...preserved..., latestMarker?, ...head..., NEW MARKER, ...tail...]
	out := make([]wingmodels.Message, 0, len(info.Messages)+1)
	out = append(out, info.Messages[:tailStart]...)
	out = append(out, markerMsg)
	out = append(out, info.Messages[tailStart:]...)

	// Emit a MessageEvent for the marker so observers (storage, UIs)
	// see it on the same channel as loop-produced messages. Without
	// this, storage sinks that listen to MessageEvent never persist
	// markers and the on-disk transcript drifts from the in-memory
	// one. Sink may be nil if the loop wasn't given one; gate the
	// emission.
	if info.Sink != nil {
		info.Sink.OnEvent(loop.MessageEvent{Message: markerMsg})
	}
	return out, nil
}

// transformContext is the read-side seam. Build the model-facing view:
// find the latest marker; replace [start..marker] with a single text
// message synthesizing all marker summaries; keep everything after.
//
// If no marker is present, return the messages unchanged.
func (p *Plugin) transformContext(_ context.Context, info loop.TransformContextInfo) ([]wingmodels.Message, error) {
	latest := findLatestMarker(info.Messages)
	if latest < 0 {
		return info.Messages, nil
	}

	// Collect every marker up to and including the latest. Concatenate
	// summaries so the model sees the full compacted history in one
	// readable block. Older markers may carry critical context the
	// latest summary alone wouldn't capture.
	var summaries []string
	for i := 0; i <= latest; i++ {
		for _, part := range info.Messages[i].Content {
			if m, ok := DecodeMarker(part); ok {
				summaries = append(summaries,
					fmt.Sprintf("[Compacted %d messages at %s]\n%s",
						m.OriginalCount, m.CompactedAt, m.Summary))
			}
		}
	}

	if len(summaries) == 0 {
		return info.Messages, nil
	}

	synth := wingmodels.Message{
		Role: wingmodels.RoleUser,
		Content: wingmodels.Content{
			wingmodels.TextPart{
				Text: "[Prior conversation summary]\n\n" + strings.Join(summaries, "\n\n"),
			},
		},
	}

	tail := info.Messages[latest+1:]
	out := make([]wingmodels.Message, 0, 1+len(tail))
	out = append(out, synth)
	out = append(out, tail...)
	return out, nil
}

// findLatestMarker returns the index of the last message whose first
// part (or any part) is a compaction marker. -1 if none.
func findLatestMarker(msgs []wingmodels.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, p := range msgs[i].Content {
			if p.Type() == PartType {
				return i
			}
		}
	}
	return -1
}

// summarize runs a single non-tool LLM call to produce a compact
// summary. Uses wingmodels.Run for sync drainage; we only want the
// final assembled text.
func summarize(ctx context.Context, model wingmodels.Model, prompt string, msgs []wingmodels.Message) (string, error) {
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}
	req := wingmodels.Request{
		System:   prompt,
		Messages: stripForSummarization(msgs),
	}
	out, err := wingmodels.Run(ctx, model, req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range out.Content {
		if tp, ok := p.(wingmodels.TextPart); ok {
			b.WriteString(tp.Text)
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "(empty summary)", nil
	}
	return s, nil
}

// stripForSummarization rewrites a message slice into a form safe to
// hand to a stateless summarization call: tool-role messages become
// user-role with inline markers; tool calls / results / reasoning
// become bracketed text; existing markers are inlined; non-text parts
// (images, etc.) are described by type.
func stripForSummarization(msgs []wingmodels.Message) []wingmodels.Message {
	out := make([]wingmodels.Message, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == wingmodels.RoleTool {
			role = wingmodels.RoleUser
		}
		var b strings.Builder
		for _, p := range m.Content {
			switch v := p.(type) {
			case wingmodels.TextPart:
				b.WriteString(v.Text)
			case wingmodels.ReasoningPart:
				b.WriteString("[reasoning] ")
				b.WriteString(truncate(v.Reasoning, 500))
			case wingmodels.ToolCallPart:
				b.WriteString(fmt.Sprintf("[tool_call %s(%s)]", v.Name, summarizeArgs(v.Input)))
			case wingmodels.ToolResultPart:
				b.WriteString(fmt.Sprintf("[tool_result %s] %s", flagError(v.IsError), truncate(extractText(v.Output), 500)))
			default:
				if mk, ok := DecodeMarker(p); ok {
					b.WriteString("[prior compaction] ")
					b.WriteString(mk.Summary)
				} else {
					b.WriteString("[" + p.Type() + "]")
				}
			}
			b.WriteString("\n")
		}
		text := strings.TrimSpace(b.String())
		if text == "" {
			continue
		}
		out = append(out, wingmodels.Message{
			Role:    role,
			Content: wingmodels.Content{wingmodels.TextPart{Text: text}},
		})
	}
	return out
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

func extractText(parts []wingmodels.Part) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(" ")
		}
		switch v := p.(type) {
		case wingmodels.TextPart:
			b.WriteString(v.Text)
		default:
			b.WriteString("[" + p.Type() + "]")
		}
	}
	return b.String()
}

func flagError(isErr bool) string {
	if isErr {
		return "(error)"
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// approxTokens estimates token count from raw text via the chars/4
// heuristic. Used only when Model.CountTokens errors.
func approxTokens(msgs []wingmodels.Message) int {
	chars := 0
	for _, m := range msgs {
		for _, p := range m.Content {
			switch v := p.(type) {
			case wingmodels.TextPart:
				chars += len(v.Text)
			case wingmodels.ReasoningPart:
				chars += len(v.Reasoning)
			case wingmodels.ToolCallPart:
				chars += len(v.Name)
				for k, val := range v.Input {
					chars += len(k) + len(fmt.Sprintf("%v", val))
				}
			case wingmodels.ToolResultPart:
				for _, op := range v.Output {
					if t, ok := op.(wingmodels.TextPart); ok {
						chars += len(t.Text)
					}
				}
			default:
				if mk, ok := DecodeMarker(p); ok {
					chars += len(mk.Summary)
				} else {
					chars += len(p.Type())
				}
			}
		}
	}
	return chars / 4
}

const defaultSummaryPrompt = `You are summarizing a long agent conversation so it can continue with bounded context. Produce a concise, faithful Markdown summary using the following sections. Omit a section if there is nothing to record there.

## Goal
What the user is trying to accomplish.

## Constraints & Preferences
Stated requirements, preferences, deadlines, style/tooling rules.

## Progress
What has been done. Use sub-bullets for: Done / In Progress / Blocked.

## Key Decisions
Important choices made and why.

## Next Steps
What should happen next.

## Critical Context
Anything else the next turn must know to continue (file paths, IDs, error messages, etc.).

## Relevant Files
Bulleted list of files touched or referenced.

Keep the summary tight; do not invent details. If information is missing, leave the section out rather than speculating.`
