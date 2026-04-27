// Package transform normalizes a request's message history for a specific
// target model before it goes on the wire.
//
// The same conversation can be replayed against any provider in wingman
// (mid-session model switching is core), but providers reject inputs they
// can't handle: Anthropic rejects empty content, OpenAI rejects orphaned
// tool calls, non-vision models reject images, reasoning blocks from one
// model are invalid on another, and turns that ended in error/aborted
// state typically have half-streamed content that fails validation.
//
// Apply runs all of the rules in order and returns a new []Message. It is
// pure: input messages are not mutated; new slices and parts are built.
//
// # Call site
//
// Each provider calls transform.Apply at the top of Stream() with its own
// Target. This keeps providers in control (they know their model's
// capabilities) and makes the transform composable with provider-specific
// pre-processing.
//
//	func (p *anthropicModel) Stream(ctx context.Context, req wingmodels.Request) (...) {
//		req.Messages = transform.Apply(req.Messages, transform.Target{
//			Provider:     "anthropic",
//			API:          wingmodels.APIAnthropicMessages,
//			ModelID:      p.id,
//			Capabilities: p.info.Capabilities,
//		})
//		// ... wire-format conversion follows
//	}
//
// # Rules (in order)
//
//  1. Drop assistant messages with FinishReason error/aborted. Their content
//     is typically half-streamed (incomplete tool calls, empty text blocks,
//     reasoning without a following message) and providers reject it. The
//     model should retry from the last clean state.
//
//  2. Cross-model reasoning handling. ReasoningParts carry provider-specific
//     signatures (Anthropic thinking signatures, OpenAI encrypted reasoning
//     content) that are only valid when replayed to the same provider+API+
//     model. When Origin.SameModel(target) is false, ReasoningParts are
//     dropped from assistant messages.
//
//  3. Image downgrade. When the target model lacks image support, ImageParts
//     in user and tool messages are replaced with a TextPart placeholder.
//     Adjacent images collapse into a single placeholder.
//
//  4. Orphan tool-call reconciliation. If an assistant message contains a
//     ToolCallPart with no matching ToolResultPart in the conversation tail
//     (before the next user message or end-of-history), a synthetic error
//     ToolResultPart is inserted. This satisfies provider validation
//     (Anthropic + OpenAI both 400 on orphans) without rewriting history.
//
//  5. Empty-content elision. Messages whose Content becomes empty after the
//     above transforms are dropped. Anthropic rejects empty content; other
//     providers either reject or silently misbehave.
//
// # Tool-call ID normalization
//
// Reserved as a future rule. OpenAI Responses generates 450+ char IDs that
// Anthropic rejects (^[a-zA-Z0-9_-]{1,64}$); Mistral expects 9-char alphanum.
// In v0.1 we ship only Anthropic + OpenAI Responses + OpenCode Zen and the
// IDs round-trip cleanly within each. NormalizeToolCallID is plumbed through
// Target as a hook so it can be added without an API break.
package transform

import (
	"github.com/chaserensberger/wingman/wingmodels"
)

// Target describes the model the transformed messages will be sent to.
// All fields are optional; zero values mean "no constraint imposed".
type Target struct {
	// Provider, API, ModelID identify the target. Used to compare against
	// each assistant message's Origin (see wingmodels.MessageOrigin) to
	// detect same-model replay. When all three match an assistant
	// message's origin, lossy normalizations (reasoning drop) are skipped
	// for that message.
	Provider string
	API      wingmodels.API
	ModelID  string

	// Capabilities describes what the target model can accept and produce.
	// transform.Apply consults Capabilities.Images to decide whether to
	// preserve or replace ImageParts. Other capability fields are reserved
	// for future rules.
	Capabilities wingmodels.ModelCapabilities

	// NormalizeToolCallID, if non-nil, is invoked on each ToolCallPart
	// CallID when the call's source assistant message did not originate
	// on this target. It returns the normalized ID. The same mapping is
	// applied to matching ToolResultParts. Reserved for future providers;
	// nil in v0.1.
	NormalizeToolCallID func(id string) string
}

// origin returns the target's identity as a MessageOrigin for SameModel
// comparisons. All three fields must be non-empty for a meaningful match;
// otherwise returns nil and SameModel will report false (cannot prove same).
func (t Target) origin() *wingmodels.MessageOrigin {
	if t.Provider == "" || t.API == "" || t.ModelID == "" {
		return nil
	}
	return &wingmodels.MessageOrigin{
		Provider: t.Provider,
		API:      t.API,
		ModelID:  t.ModelID,
	}
}

const (
	// Placeholders shown to non-vision models in place of stripped images.
	// Distinct strings for user vs. tool images so the model can tell which
	// came from where if it asks. Matches pi-ai's wording.
	placeholderUserImage = "(image omitted: model does not support images)"
	placeholderToolImage = "(tool image omitted: model does not support images)"

	// Synthesized when an assistant tool call has no matching result.
	// Marked IsError so the model recognizes it as a failed call rather
	// than a real outcome.
	syntheticOrphanResult = "No result provided"
)

// Apply runs all transform rules in order against msgs and returns a new
// slice. msgs is not mutated. Always returns a non-nil slice (possibly
// empty); empty input returns an empty result.
//
// Order of operations is deliberate: error/aborted pruning runs first so
// later rules don't waste work on dropped messages; reasoning drop runs
// before image downgrade because both are per-message rewrites; orphan
// reconciliation runs last because it depends on the final assistant/tool
// message structure after the per-message rewrites.
func Apply(msgs []wingmodels.Message, target Target) []wingmodels.Message {
	if len(msgs) == 0 {
		return []wingmodels.Message{}
	}

	// 1. Drop errored/aborted assistant turns.
	pruned := dropFailedAssistants(msgs)

	// Build the tool-call ID rename map. Only call IDs from cross-model
	// assistant messages get renamed. The map is then applied to both
	// the assistant tool calls and the matching tool results so they
	// stay paired. When NormalizeToolCallID is nil the map is empty
	// (no work in the per-message pass).
	tgtOrigin := target.origin()
	idRenames := buildToolCallIDRenames(pruned, target, tgtOrigin)

	// 2 + 3. Per-message rewrites: reasoning drop (cross-model only) and
	// image downgrade (when target lacks image support). Combined into one
	// pass since both produce a new Content slice.
	rewritten := make([]wingmodels.Message, 0, len(pruned))
	for _, msg := range pruned {
		rewritten = append(rewritten, rewriteMessage(msg, target, tgtOrigin, idRenames))
	}

	// 4. Orphan tool-call reconciliation.
	withResults := reconcileOrphanedToolCalls(rewritten)

	// 5. Drop messages whose content is empty after rewrites.
	out := make([]wingmodels.Message, 0, len(withResults))
	for _, msg := range withResults {
		if len(msg.Content) > 0 {
			out = append(out, msg)
		}
	}
	return out
}

// buildToolCallIDRenames walks the message list and produces a map from
// original CallID to normalized CallID for every cross-model assistant
// tool call, when the target has a NormalizeToolCallID hook. Same-model
// tool calls are not normalized (their IDs are already valid for the
// target). Returns an empty map when there is nothing to do.
func buildToolCallIDRenames(msgs []wingmodels.Message, target Target, tgtOrigin *wingmodels.MessageOrigin) map[string]string {
	if target.NormalizeToolCallID == nil {
		return nil
	}
	out := map[string]string{}
	for _, msg := range msgs {
		if msg.Role != wingmodels.RoleAssistant {
			continue
		}
		if msg.Origin.SameModel(tgtOrigin) {
			continue
		}
		for _, p := range msg.Content {
			tc, ok := p.(wingmodels.ToolCallPart)
			if !ok {
				continue
			}
			normalized := target.NormalizeToolCallID(tc.CallID)
			if normalized != tc.CallID {
				out[tc.CallID] = normalized
			}
		}
	}
	return out
}

// dropFailedAssistants removes assistant messages whose FinishReason is
// error or aborted. Non-assistant messages always pass through.
//
// Important caveat: this also drops the failed turn's *subsequent* tool
// results, because they reference tool calls in the dropped assistant
// message. If we kept them, the next pass would see orphaned results
// (no matching call) which providers also reject. Tool-result messages
// emitted while the failed assistant was still streaming are tied to that
// assistant's tool calls; their CallIDs won't match any surviving
// assistant message and the orphan-reconciliation pass would not insert
// anything for them, but the providers see them as dangling references.
//
// We handle this by tracking the dropped assistant's tool-call IDs and
// also dropping any tool-role messages whose CallID is in that set, until
// the next user message resets state.
func dropFailedAssistants(msgs []wingmodels.Message) []wingmodels.Message {
	out := make([]wingmodels.Message, 0, len(msgs))
	droppedCallIDs := map[string]struct{}{}

	for _, msg := range msgs {
		switch msg.Role {
		case wingmodels.RoleAssistant:
			if msg.FinishReason == wingmodels.FinishReasonError ||
				msg.FinishReason == wingmodels.FinishReasonAborted {
				// Record this assistant's tool-call IDs so we can drop
				// any straggler results that reference them.
				for _, p := range msg.Content {
					if tc, ok := p.(wingmodels.ToolCallPart); ok {
						droppedCallIDs[tc.CallID] = struct{}{}
					}
				}
				continue
			}
			// Successful assistant; nothing to skip.
			out = append(out, msg)
		case wingmodels.RoleTool:
			// Drop tool results whose call belongs to a dropped assistant.
			drop := false
			for _, p := range msg.Content {
				if tr, ok := p.(wingmodels.ToolResultPart); ok {
					if _, dead := droppedCallIDs[tr.CallID]; dead {
						drop = true
						break
					}
				}
			}
			if drop {
				continue
			}
			out = append(out, msg)
		case wingmodels.RoleUser:
			// User messages reset the dropped-call tracking — we're past
			// the failed turn now.
			droppedCallIDs = map[string]struct{}{}
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	return out
}

// rewriteMessage applies per-message transforms to a single message:
// cross-model reasoning drop and tool-call ID rename (assistant only),
// image downgrade (user/tool only), and tool-result ID rename (tool only).
// Returns a new Message; input is not mutated.
func rewriteMessage(
	msg wingmodels.Message,
	target Target,
	tgtOrigin *wingmodels.MessageOrigin,
	idRenames map[string]string,
) wingmodels.Message {
	switch msg.Role {
	case wingmodels.RoleAssistant:
		sameModel := msg.Origin.SameModel(tgtOrigin)
		// Fast path: same-model assistant with nothing to rewrite.
		if sameModel && len(idRenames) == 0 {
			return msg
		}
		newContent := make(wingmodels.Content, 0, len(msg.Content))
		for _, p := range msg.Content {
			switch part := p.(type) {
			case wingmodels.ReasoningPart:
				if sameModel {
					newContent = append(newContent, part)
				}
				// else: drop (no fallback to text — reasoning is a
				// distinct content kind and providers expect it absent
				// rather than reformatted, matches AI SDK behavior).
			case wingmodels.ToolCallPart:
				if !sameModel {
					if newID, ok := idRenames[part.CallID]; ok {
						part.CallID = newID
					}
				}
				newContent = append(newContent, part)
			default:
				newContent = append(newContent, p)
			}
		}
		out := msg
		out.Content = newContent
		return out

	case wingmodels.RoleUser:
		if target.Capabilities.Images {
			return msg
		}
		newContent, changed := downgradeImages(msg.Content, placeholderUserImage)
		if !changed {
			return msg
		}
		out := msg
		out.Content = newContent
		return out

	case wingmodels.RoleTool:
		// Tool messages may need image downgrade inside ToolResultPart.Output
		// and/or CallID rename to match a renamed assistant tool call.
		needsImage := !target.Capabilities.Images
		needsRename := len(idRenames) > 0
		if !needsImage && !needsRename {
			return msg
		}
		newContent := make(wingmodels.Content, 0, len(msg.Content))
		changed := false
		for _, p := range msg.Content {
			tr, ok := p.(wingmodels.ToolResultPart)
			if !ok {
				newContent = append(newContent, p)
				continue
			}
			if needsImage {
				if down, dchanged := downgradeImages(tr.Output, placeholderToolImage); dchanged {
					tr.Output = down
					changed = true
				}
			}
			if needsRename {
				if newID, ok := idRenames[tr.CallID]; ok {
					tr.CallID = newID
					changed = true
				}
			}
			newContent = append(newContent, tr)
		}
		if !changed {
			return msg
		}
		out := msg
		out.Content = newContent
		return out
	}
	return msg
}

// downgradeImages walks a Content slice and replaces each ImagePart with a
// TextPart placeholder. Adjacent images (or images adjacent to an existing
// placeholder text) collapse into a single placeholder so the model isn't
// flooded with identical strings. Returns (content, changed); when changed
// is false the caller should keep the original slice (no images were
// present).
func downgradeImages(content []wingmodels.Part, placeholder string) ([]wingmodels.Part, bool) {
	hasImage := false
	for _, p := range content {
		if _, ok := p.(wingmodels.ImagePart); ok {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return content, false
	}

	out := make([]wingmodels.Part, 0, len(content))
	prevWasPlaceholder := false
	for _, p := range content {
		if _, ok := p.(wingmodels.ImagePart); ok {
			if !prevWasPlaceholder {
				out = append(out, wingmodels.TextPart{Text: placeholder})
			}
			prevWasPlaceholder = true
			continue
		}
		out = append(out, p)
		// Only treat the literal placeholder string as a "previous
		// placeholder" for collapse purposes — any other text resets.
		if tp, ok := p.(wingmodels.TextPart); ok && tp.Text == placeholder {
			prevWasPlaceholder = true
		} else {
			prevWasPlaceholder = false
		}
	}
	return out, true
}

// reconcileOrphanedToolCalls walks the message list and inserts a synthetic
// error ToolResultPart for any assistant ToolCallPart that lacks a matching
// ToolResultPart before the next user message or end-of-history.
//
// "Matching" = same CallID. Tool result CallIDs are tracked since the most
// recent assistant message; when a user message arrives or the list ends,
// any tracked-but-unmatched calls get synthesized results inserted at the
// current position (before the user message, or appended at the end).
//
// Tool calls from earlier assistant messages whose results arrived after a
// later assistant turn started are not synthesized — that's a malformed
// conversation we don't try to repair.
func reconcileOrphanedToolCalls(msgs []wingmodels.Message) []wingmodels.Message {
	out := make([]wingmodels.Message, 0, len(msgs))

	// Pending = tool calls from the most recent assistant message that
	// haven't yet seen a result. Reset whenever we see a user message
	// (next-turn boundary) or another assistant message.
	type pending struct {
		callID   string
		toolName string
	}
	var pendingCalls []pending
	resultsSeen := map[string]struct{}{}

	flushSynthetic := func() {
		for _, pc := range pendingCalls {
			if _, ok := resultsSeen[pc.callID]; ok {
				continue
			}
			out = append(out, wingmodels.NewToolResult(
				pc.callID,
				[]wingmodels.Part{wingmodels.TextPart{Text: syntheticOrphanResult}},
				true,
			))
		}
		pendingCalls = nil
		resultsSeen = map[string]struct{}{}
	}

	for _, msg := range msgs {
		switch msg.Role {
		case wingmodels.RoleAssistant:
			// Before processing this assistant turn, close out any
			// pending calls from the previous one.
			flushSynthetic()
			out = append(out, msg)
			for _, p := range msg.Content {
				if tc, ok := p.(wingmodels.ToolCallPart); ok {
					pendingCalls = append(pendingCalls, pending{
						callID:   tc.CallID,
						toolName: tc.Name,
					})
				}
			}
		case wingmodels.RoleTool:
			out = append(out, msg)
			for _, p := range msg.Content {
				if tr, ok := p.(wingmodels.ToolResultPart); ok {
					resultsSeen[tr.CallID] = struct{}{}
				}
			}
		case wingmodels.RoleUser:
			// User message ends the prior assistant's tool window.
			flushSynthetic()
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	// Tail: synthesize for anything still pending at end-of-history.
	flushSynthetic()
	return out
}
