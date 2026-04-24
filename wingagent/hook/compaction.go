// Package hook ships ready-to-use implementations of the loop's
// lifecycle hook seams. Each constructor returns a value matching one of
// the loop's hook function types (e.g. loop.BeforeStepHook), suitable
// for plugging into loop.Config.Hooks or session.WithBeforeStep / etc.
//
// Hooks shipped here are deliberately small and orthogonal — composition
// happens at install time. If two hooks need to interleave, write a
// thin wrapper that calls them in order; the loop itself only allows
// one hook per seam by design (keeps the call site obvious).
//
// Adding a new hook:
//  1. New file <name>.go in this package.
//  2. Export a constructor returning the appropriate loop hook type.
//  3. Use functional options (Compaction is the canonical pattern).
//  4. Keep all helpers unexported; consumers only need the constructor.
package hook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingmodels"
)

// CompactionOption configures Compaction. Use With* helpers below; do
// not zero-init the underlying struct directly (it's intentionally
// unexported so future fields don't break callers).
type CompactionOption func(*compactionConfig)

type compactionConfig struct {
	// threshold is the input-tokens-to-context-window ratio at which
	// compaction triggers. 0.85 means "compact when 85% full." Anything
	// >= 1 effectively disables triggering; anything <= 0 always
	// triggers (probably what you want only in tests).
	threshold float64

	// keepTail is the number of trailing messages to leave intact after
	// compaction. The head — everything before the kept tail — is
	// summarized into a single CompactionMarkerPart message.
	keepTail int

	// minMessages is the floor below which compaction never runs, even
	// if threshold is exceeded. Prevents pathological "compact a 3-msg
	// history" behavior on tiny-context-window models.
	minMessages int

	// summaryPrompt is the system prompt used for the summarization
	// sub-call. Empty means defaultSummaryPrompt.
	summaryPrompt string

	// model overrides the model used for the summarization sub-call.
	// nil means "use the loop's model" (read from BeforeStepInfo.Model
	// at hook invocation time, so SetModel-style swaps are honored).
	model wingmodels.Model
}

// WithThreshold sets the input-tokens / context-window ratio that
// triggers compaction. Default 0.85.
func WithThreshold(f float64) CompactionOption {
	return func(c *compactionConfig) { c.threshold = f }
}

// WithKeepTail sets how many trailing messages survive compaction
// untouched. Default 4.
func WithKeepTail(n int) CompactionOption {
	return func(c *compactionConfig) { c.keepTail = n }
}

// WithMinMessages sets the floor below which compaction never runs.
// Default 6. Setting this to 0 effectively disables the floor.
func WithMinMessages(n int) CompactionOption {
	return func(c *compactionConfig) { c.minMessages = n }
}

// WithSummaryPrompt overrides the summarization system prompt. The
// default is a structured Markdown schema mirroring opencode's pattern.
func WithSummaryPrompt(s string) CompactionOption {
	return func(c *compactionConfig) { c.summaryPrompt = s }
}

// WithCompactionModel uses a specific model for the summarization
// sub-call. Default behavior reads the loop's model at invocation time.
// Useful when summarization should run on a cheaper / faster / longer-
// context model than the main conversation.
func WithCompactionModel(m wingmodels.Model) CompactionOption {
	return func(c *compactionConfig) { c.model = m }
}

// Compaction returns a loop.BeforeStepHook that summarizes the head of
// the message slice when input-token usage approaches the model's
// context window. The hook returns a slice whose first message contains
// a wingmodels.CompactionMarkerPart with the summary, followed by the
// trailing keepTail messages.
//
// The hook is a no-op when:
//   - the model's context window is unknown (Model.Info().ContextWindow == 0)
//   - len(messages) < minMessages
//   - usage.InputTokens / contextWindow < threshold
//   - the summarization sub-call fails (returns the error; loop will
//     fail the run)
//
// Defaults: threshold 0.85, keepTail 4, minMessages 6.
func Compaction(opts ...CompactionOption) loop.BeforeStepHook {
	cfg := compactionConfig{
		threshold:   0.85,
		keepTail:    4,
		minMessages: 6,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return func(ctx context.Context, info loop.BeforeStepInfo) ([]wingmodels.Message, error) {
		// Choose model: explicit override > loop's model.
		model := cfg.model
		if model == nil {
			model = info.Model
		}
		if model == nil {
			// No model anywhere; can't summarize. No-op.
			return info.Messages, nil
		}

		// Trigger gates.
		if len(info.Messages) < cfg.minMessages {
			return info.Messages, nil
		}
		ctxWindow := model.Info().ContextWindow
		if ctxWindow <= 0 {
			return info.Messages, nil
		}
		ratio := float64(info.Usage.InputTokens) / float64(ctxWindow)
		if ratio < cfg.threshold {
			return info.Messages, nil
		}
		if cfg.keepTail >= len(info.Messages) {
			// Nothing to summarize after keeping the tail.
			return info.Messages, nil
		}

		head := info.Messages[:len(info.Messages)-cfg.keepTail]
		tail := info.Messages[len(info.Messages)-cfg.keepTail:]

		summary, err := summarize(ctx, model, cfg.summaryPrompt, head)
		if err != nil {
			return nil, fmt.Errorf("compaction summarize: %w", err)
		}

		marker := wingmodels.Message{
			Role: wingmodels.RoleUser,
			Content: wingmodels.Content{
				wingmodels.CompactionMarkerPart{
					Summary:       summary,
					OriginalCount: len(head),
					CompactedAt:   time.Now().UTC().Format(time.RFC3339),
				},
			},
		}

		out := make([]wingmodels.Message, 0, 1+len(tail))
		out = append(out, marker)
		out = append(out, tail...)
		return out, nil
	}
}

// summarize runs a single non-tool LLM call to produce a compact
// summary of the given messages. Uses wingmodels.Run so we don't
// have to manually drain the stream — we only care about the final
// assembled message's text.
//
// We strip tool / image content first because (a) the summary doesn't
// need it and (b) some providers reject mid-conversation tool messages
// without paired calls (Anthropic, Google) once they're separated from
// their matching tool_use blocks.
func summarize(ctx context.Context, model wingmodels.Model, prompt string, msgs []wingmodels.Message) (string, error) {
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}
	req := wingmodels.Request{
		System:   prompt,
		Messages: stripForSummarization(msgs),
		// No tools — summarization is text-only.
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
// hand to a stateless summarization call: tool-role messages are
// converted to user-role with an inline marker; tool calls / results
// inside content become bracketed text; reasoning becomes bracketed
// text; existing compaction markers are inlined; non-text parts (e.g.
// images) are dropped.
func stripForSummarization(msgs []wingmodels.Message) []wingmodels.Message {
	out := make([]wingmodels.Message, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		// Re-roling tool messages to user keeps them inside the
		// alternating-role contract some providers enforce.
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
			case wingmodels.CompactionMarkerPart:
				b.WriteString("[prior compaction] ")
				b.WriteString(v.Summary)
			default:
				// Drop unknown / image / file parts.
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

// extractText pulls a flat text representation out of a ToolResultPart's
// Output slice — only TextPart contributes; non-text parts (images, etc.)
// are described by type so the summary still records that they existed.
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

// defaultSummaryPrompt asks for a structured Markdown summary mirroring
// opencode's compaction prompt (trimmed). The structure helps the next
// model turn pick up context without re-reading the full transcript.
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
