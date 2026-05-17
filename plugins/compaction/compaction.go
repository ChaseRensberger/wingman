// Package compaction is the canonical agent plugin: it summarizes
// the head of a long message history into a single inline marker so
// long-running sessions stay under the model's context window without
// losing ground-truth on disk.
//
// # Design
//
// Compaction has two halves:
//
//   - Write-side (TransformHistory): when input tokens approach the model's
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
// Single-seam approaches (truncate-and-replace in TransformHistory) lose
// history irrecoverably and prevent UIs from showing what was
// compacted. Splitting write (append marker) from read (filter) keeps
// every byte addressable and lets observability surfaces render the
// pre-compaction transcript verbatim.
//
// # Token estimation
//
// The hook estimates tokens from the current message snapshot via a chars/4
// heuristic. That avoids first-call blindness and lag-by-one behavior from
// relying on the previous turn's usage report.
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
//	    compaction.WithKeepRecentTokens(12000),
//	))
package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/models"
)

// PartType is the discriminator string MarkerPart serializes with.
// Stable; persisted in storage and on the SSE wire. The session stream
// classifier inspects this string to surface the dedicated "compaction"
// SSE event type without importing this package.
const PartType = "compaction_marker"

// MarkerPart records that a span of conversation history was
// summarized. Inserted by the plugin's TransformHistory hook in append
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
	// FirstKeptIndex is the message index where uncompacted context resumes.
	// Wingman messages do not have durable entry IDs at this hook seam, so this
	// is diagnostic metadata rather than a stable replay pointer.
	FirstKeptIndex int `json:"first_kept_index,omitempty"`
	// TokensBefore is the approximate model-facing input token count that
	// triggered compaction.
	TokensBefore int `json:"tokens_before,omitempty"`
	// ReadFiles and ModifiedFiles are cumulative file-operation hints extracted
	// from summarized tool calls and prior compaction markers.
	ReadFiles     []string `json:"read_files,omitempty"`
	ModifiedFiles []string `json:"modified_files,omitempty"`
}

func (MarkerPart) Type() string { return PartType }

// MarshalJSON / UnmarshalJSON: defaults via field tags are sufficient.
// The models.Part interface's unexported isPart marker means
// MarkerPart cannot satisfy Part by name from outside models. We
// route through models.OpaquePart at the registry seam: the part
// type is registered with a decoder that returns a *typed* part wrapped
// in an adapter, but the adapter still needs to satisfy Part.
//
// Workaround: the Plugin's RegisterPart decoder returns a
// models.OpaquePart whose Raw is the marker's JSON. Read-side
// callers that want typed access call DecodeMarker(part) which extracts
// MarkerPart from the OpaquePart's bytes. This keeps models' Part
// union sealed (no external types satisfy it directly) while letting
// plugins ship "logical" Part types over the OpaquePart carrier.

// DecodeMarker extracts a MarkerPart from a models.Part if it
// represents a compaction marker. Returns ok=false for any other
// part. Safe to call on every part during a content walk.
func DecodeMarker(p models.Part) (MarkerPart, bool) {
	if p == nil || p.Type() != PartType {
		return MarkerPart{}, false
	}
	op, ok := p.(models.OpaquePart)
	if !ok {
		return MarkerPart{}, false
	}
	var m MarkerPart
	if err := json.Unmarshal(op.Raw, &m); err != nil {
		return MarkerPart{}, false
	}
	return m, true
}

// newMarkerPart constructs a models.Part carrying a MarkerPart's
// payload. Implemented as an OpaquePart so it satisfies models.Part
// without breaking the sealed-union invariant.
func newMarkerPart(m MarkerPart) (models.Part, error) {
	body, err := json.Marshal(struct {
		Type           string   `json:"type"`
		Summary        string   `json:"summary"`
		OriginalCount  int      `json:"original_count"`
		CompactedAt    string   `json:"compacted_at"`
		FirstKeptIndex int      `json:"first_kept_index,omitempty"`
		TokensBefore   int      `json:"tokens_before,omitempty"`
		ReadFiles      []string `json:"read_files,omitempty"`
		ModifiedFiles  []string `json:"modified_files,omitempty"`
	}{PartType, m.Summary, m.OriginalCount, m.CompactedAt, m.FirstKeptIndex, m.TokensBefore, m.ReadFiles, m.ModifiedFiles})
	if err != nil {
		return nil, err
	}
	return models.OpaquePart{TypeName: PartType, Raw: body}, nil
}

// Option configures a Plugin.
type Option func(*Plugin)

// Plugin is the compaction plugin instance.
type Plugin struct {
	threshold     float64
	keepTail      int
	keepRecent    int
	reserveTokens int
	minMessages   int
	summaryPrompt string
	client        models.Client
	model         models.ModelRef
	modelInfo     models.ModelInfo
}

// New constructs a compaction plugin with the supplied options applied
// over the defaults: threshold 0.85, keepRecent 20k tokens, reserve 16k
// tokens, minMessages 6.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		threshold:     0.85,
		keepTail:      4,
		keepRecent:    20000,
		reserveTokens: 16384,
		minMessages:   6,
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
// untouched and disables token-budget tail selection. Prefer
// WithKeepRecentTokens for Pi-style behavior.
func WithKeepTail(n int) Option { return func(p *Plugin) { p.keepTail, p.keepRecent = n, 0 } }

// WithKeepRecentTokens sets the approximate recent-token budget that survives
// compaction untouched. Default 20000.
func WithKeepRecentTokens(n int) Option { return func(p *Plugin) { p.keepRecent = n } }

// WithReserveTokens sets the approximate response-token buffer that triggers
// compaction before the context window is full. Default 16384; ignored when it
// is greater than or equal to the model context window.
func WithReserveTokens(n int) Option { return func(p *Plugin) { p.reserveTokens = n } }

// WithMinMessages sets the floor below which compaction never runs.
// Default 6. Setting this to 0 disables the floor.
func WithMinMessages(n int) Option { return func(p *Plugin) { p.minMessages = n } }

// WithSummaryPrompt overrides the summarization system prompt.
func WithSummaryPrompt(s string) Option { return func(p *Plugin) { p.summaryPrompt = s } }

// WithModelRef uses a specific model for the summarization sub-call.
// Default: use the loop's model at invocation time. Useful when
// summarization should run on a cheaper / faster / longer-context
// model than the main conversation.
func WithModelRef(client models.Client, ref models.ModelRef, info models.ModelInfo) Option {
	return func(p *Plugin) {
		p.client = client
		p.model = ref
		p.modelInfo = info
	}
}

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return "compaction" }

// Install implements plugin.Plugin. Registers the marker Part decoder,
// the TransformHistory write-side hook, and the TransformContext read-side
// filter.
func (p *Plugin) Install(r *plugin.Registry) error {
	// Part decoder: return an OpaquePart preserving the bytes. The
	// payload is small and DecodeMarker re-parses on demand; storing
	// raw bytes avoids needing a models.Part-satisfying typed
	// wrapper (the Part union is sealed to models).
	r.RegisterPart(PartType, func(data []byte) (models.Part, error) {
		raw := make([]byte, len(data))
		copy(raw, data)
		return models.OpaquePart{TypeName: PartType, Raw: raw}, nil
	})

	r.RegisterTransformHistory(p.transformHistory)
	r.RegisterTransformContext(p.transformContext)
	return nil
}

// transformHistory is the write-side seam. When token usage crosses the
// threshold, summarize every message after the most recent marker (or
// from the start, if none) up to the keepTail boundary, then append a
// new marker. The pre-compaction messages are kept in history so the
// transcript remains addressable on disk and via History().
func (p *Plugin) transformHistory(ctx context.Context, info loop.TransformHistoryInfo) ([]models.Message, error) {
	client := p.client
	model := p.model
	modelInfo := p.modelInfo
	if client == nil {
		client = info.Client
		model = info.Model
		modelInfo = info.ModelInfo
	}
	if client == nil || model.Provider == "" || model.ID == "" {
		return info.Messages, nil
	}

	if len(info.Messages) < p.minMessages {
		return info.Messages, nil
	}
	ctxWindow := modelInfo.ContextWindow
	if ctxWindow <= 0 {
		return info.Messages, nil
	}

	tokens := approxTokens(info.Messages)
	triggerTokens := int(float64(ctxWindow) * p.threshold)
	if p.reserveTokens > 0 && p.reserveTokens < ctxWindow {
		reservedTrigger := ctxWindow - p.reserveTokens
		if reservedTrigger < triggerTokens {
			triggerTokens = reservedTrigger
		}
	}
	if tokens < triggerTokens {
		return info.Messages, nil
	}

	// Find the latest marker so we don't re-summarize what's already
	// summarized. We summarize messages in [latestMarkerIdx+1, tailStart).
	latestMarkerIdx := findLatestMarker(info.Messages)
	tailStart := p.findTailStart(info.Messages, latestMarkerIdx+1, triggerTokens)
	if tailStart <= latestMarkerIdx+1 {
		// Nothing new to summarize since the last marker.
		return info.Messages, nil
	}

	headStart := latestMarkerIdx + 1
	head := info.Messages[headStart:tailStart]
	if len(head) == 0 {
		return info.Messages, nil
	}

	previous, readFiles, modifiedFiles := collectMarkerState(info.Messages[:headStart])
	readNow, modifiedNow := collectFileOps(head)
	readFiles = mergeStrings(readFiles, readNow)
	modifiedFiles = mergeStrings(modifiedFiles, modifiedNow)

	summary, err := summarize(ctx, client, model, p.summaryPrompt, previous, head)
	if err != nil {
		return nil, fmt.Errorf("summarize: %w", err)
	}

	markerPart, err := newMarkerPart(MarkerPart{
		Summary:        summary,
		OriginalCount:  len(head),
		CompactedAt:    time.Now().UTC().Format(time.RFC3339),
		FirstKeptIndex: tailStart,
		TokensBefore:   tokens,
		ReadFiles:      readFiles,
		ModifiedFiles:  modifiedFiles,
	})
	if err != nil {
		return nil, fmt.Errorf("build marker: %w", err)
	}

	markerMsg := models.Message{
		Role:    models.RoleUser,
		Content: models.Content{markerPart},
	}

	// Append marker between head and tail. Result:
	//   [...preserved..., latestMarker?, ...head..., NEW MARKER, ...tail...]
	out := make([]models.Message, 0, len(info.Messages)+1)
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
func (p *Plugin) transformContext(_ context.Context, info loop.TransformContextInfo) ([]models.Message, error) {
	latest := findLatestMarker(info.Messages)
	if latest < 0 {
		return info.Messages, nil
	}

	marker, ok := markerInMessage(info.Messages[latest])
	if !ok {
		return info.Messages, nil
	}

	synth := models.Message{
		Role: models.RoleUser,
		Content: models.Content{
			models.TextPart{
				Text: fmt.Sprintf("[Prior conversation summary]\n\n[Compacted %d messages at %s]\n%s",
					marker.OriginalCount, marker.CompactedAt, marker.Summary),
			},
		},
	}

	tail := info.Messages[latest+1:]
	out := make([]models.Message, 0, 1+len(tail))
	out = append(out, synth)
	out = append(out, tail...)
	return out, nil
}

// findLatestMarker returns the index of the last message whose first
// part (or any part) is a compaction marker. -1 if none.
func findLatestMarker(msgs []models.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, p := range msgs[i].Content {
			if p.Type() == PartType {
				return i
			}
		}
	}
	return -1
}

func markerInMessage(msg models.Message) (MarkerPart, bool) {
	for _, p := range msg.Content {
		if m, ok := DecodeMarker(p); ok {
			return m, true
		}
	}
	return MarkerPart{}, false
}

func (p *Plugin) findTailStart(msgs []models.Message, minStart int, triggerTokens int) int {
	if p.keepRecent <= 0 {
		return safeTailStartByMessages(msgs, minStart, p.keepTail)
	}

	keepRecent := p.keepRecent
	if triggerTokens > 0 && keepRecent >= triggerTokens {
		keepRecent = triggerTokens / 2
	}
	if keepRecent < 1 {
		keepRecent = 1
	}

	tokens := 0
	candidate := len(msgs)
	for i := len(msgs) - 1; i >= minStart; i-- {
		tokens += approxMessageTokens(msgs[i])
		candidate = i
		if tokens >= keepRecent {
			break
		}
	}
	return safeTailStartAtOrAfter(msgs, minStart, candidate)
}

func safeTailStartByMessages(msgs []models.Message, minStart int, keepTail int) int {
	if keepTail < 0 {
		keepTail = 0
	}
	candidate := len(msgs) - keepTail
	if candidate < minStart {
		candidate = minStart
	}
	return safeTailStartAtOrAfter(msgs, minStart, candidate)
}

func safeTailStartAtOrAfter(msgs []models.Message, minStart int, candidate int) int {
	if candidate < minStart {
		candidate = minStart
	}
	for i := candidate; i < len(msgs); i++ {
		if isSafeFirstKept(msgs, i) {
			return i
		}
	}
	return len(msgs)
}

func isSafeFirstKept(msgs []models.Message, i int) bool {
	if i <= 0 || i >= len(msgs) {
		return i == 0 || i == len(msgs)
	}
	if msgs[i].Role == models.RoleTool {
		return false
	}
	if msgHasToolResult(msgs[i]) {
		return false
	}
	if msgHasToolCall(msgs[i-1]) {
		return false
	}
	return true
}

func msgHasToolCall(msg models.Message) bool {
	for _, p := range msg.Content {
		if _, ok := p.(models.ToolCallPart); ok {
			return true
		}
	}
	return false
}

func msgHasToolResult(msg models.Message) bool {
	for _, p := range msg.Content {
		if _, ok := p.(models.ToolResultPart); ok {
			return true
		}
	}
	return false
}

// summarize runs a single non-tool LLM call to produce a compact
// summary. Uses models.Run for sync drainage; we only want the
// final assembled text.
func summarize(ctx context.Context, client models.Client, model models.ModelRef, prompt string, previous string, msgs []models.Message) (string, error) {
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}
	summaryMsgs := stripForSummarization(msgs)
	if previous != "" {
		summaryMsgs = append([]models.Message{{
			Role:    models.RoleUser,
			Content: models.Content{models.TextPart{Text: "[Previous compaction summary]\n\n" + previous}},
		}}, summaryMsgs...)
	}
	req := models.Request{
		Model:    model,
		System:   prompt,
		Messages: summaryMsgs,
	}
	out, err := client.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range out.Content {
		if tp, ok := p.(models.TextPart); ok {
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
func stripForSummarization(msgs []models.Message) []models.Message {
	out := make([]models.Message, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == models.RoleTool {
			role = models.RoleUser
		}
		var b strings.Builder
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				b.WriteString(v.Text)
			case models.ReasoningPart:
				b.WriteString("[reasoning] ")
				b.WriteString(truncate(v.Reasoning, 500))
			case models.ToolCallPart:
				b.WriteString(fmt.Sprintf("[tool_call %s(%s)]", v.Name, summarizeArgs(v.Input)))
			case models.ToolResultPart:
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
		out = append(out, models.Message{
			Role:    role,
			Content: models.Content{models.TextPart{Text: text}},
		})
	}
	return out
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	body, err := json.Marshal(args)
	if err == nil {
		return truncate(string(body), 500)
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func extractText(parts []models.Part) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(" ")
		}
		switch v := p.(type) {
		case models.TextPart:
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

func collectMarkerState(msgs []models.Message) (summary string, readFiles []string, modifiedFiles []string) {
	var summaries []string
	for _, msg := range msgs {
		for _, part := range msg.Content {
			m, ok := DecodeMarker(part)
			if !ok {
				continue
			}
			summaries = append(summaries, fmt.Sprintf("[Compacted %d messages at %s]\n%s", m.OriginalCount, m.CompactedAt, m.Summary))
			readFiles = mergeStrings(readFiles, m.ReadFiles)
			modifiedFiles = mergeStrings(modifiedFiles, m.ModifiedFiles)
		}
	}
	return strings.Join(summaries, "\n\n"), readFiles, modifiedFiles
}

func collectFileOps(msgs []models.Message) (readFiles []string, modifiedFiles []string) {
	for _, msg := range msgs {
		for _, part := range msg.Content {
			call, ok := part.(models.ToolCallPart)
			if !ok {
				continue
			}
			path := stringArg(call.Input, "path")
			switch call.Name {
			case "read", "grep", "glob":
				readFiles = appendIfNonEmpty(readFiles, path)
			case "edit", "write":
				modifiedFiles = appendIfNonEmpty(modifiedFiles, path)
			}
		}
	}
	return uniqueSorted(readFiles), uniqueSorted(modifiedFiles)
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func appendIfNonEmpty(vals []string, v string) []string {
	if strings.TrimSpace(v) == "" {
		return vals
	}
	return append(vals, v)
}

func mergeStrings(a, b []string) []string {
	return uniqueSorted(append(append([]string(nil), a...), b...))
}

func uniqueSorted(vals []string) []string {
	seen := make(map[string]bool, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// approxTokens estimates token count from raw text via the chars/4
// heuristic. Used only when Model.CountTokens errors.
func approxTokens(msgs []models.Message) int {
	chars := 0
	for _, m := range msgs {
		chars += approxMessageChars(m)
	}
	return chars / 4
}

func approxMessageTokens(msg models.Message) int {
	return approxMessageChars(msg) / 4
}

func approxMessageChars(msg models.Message) int {
	chars := 0
	for _, p := range msg.Content {
		switch v := p.(type) {
		case models.TextPart:
			chars += len(v.Text)
		case models.ReasoningPart:
			chars += len(v.Reasoning)
		case models.ToolCallPart:
			chars += len(v.Name)
			for k, val := range v.Input {
				chars += len(k) + len(fmt.Sprintf("%v", val))
			}
		case models.ToolResultPart:
			for _, op := range v.Output {
				if t, ok := op.(models.TextPart); ok {
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
	return chars
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

<read-files>
One referenced/read path per line, if known.
</read-files>

<modified-files>
One modified path per line, if known.
</modified-files>

Keep the summary tight; do not invent details. If information is missing, leave the section out rather than speculating.`
