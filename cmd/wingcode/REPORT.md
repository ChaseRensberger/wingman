# WingCode — Wingman Limitations Report

This document lists features from OpenCode's TUI that could not be replicated in WingCode due to current limitations in the Wingman SDK/server, along with suggestions for improvements.

## Critical Limitations

### 1. No Session Abort/Interrupt
**OpenCode:** `POST /session/abort` cancels a running inference immediately.
**Wingman:** No abort endpoint. Closing the SSE connection doesn't stop the server-side agentic loop — it continues running tools and making inference calls.
**Impact:** Users press Esc expecting to interrupt, but the server keeps running. This wastes API credits and can cause unwanted file modifications.
**Suggestion:** Add a `POST /sessions/{id}/abort` endpoint that cancels the context passed to `session.RunStream()`. Store a cancel function per active session.

### 2. No Tool Result Events in Stream
**OpenCode:** Emits `tool_result` events during streaming so the TUI can show tool execution results in real-time.
**Wingman:** The `SessionStream` only forwards provider-level stream events (`text_delta`, `content_block_start`, `input_json_delta`, etc.). Tool execution happens silently between inference rounds. The TUI sees a tool call start, then nothing until the next inference round begins.
**Impact:** Tool calls appear to hang — the user sees "Using bash..." but never sees the result until the assistant's next text response.
**Suggestion:** Emit synthetic `tool_result` events from `SessionStream` after each tool execution completes. Add a new event type to `models.StreamEventType`:
```go
EventToolResult StreamEventType = "tool_result"
```
Emit it from the background goroutine in `session/stream.go` after `tool.Execute()` returns.

### 3. No Token Cost Tracking
**OpenCode:** Tracks per-message cost, cache read/write tokens, reasoning tokens. Displays running cost in header.
**Wingman:** `WingmanUsage` only has `InputTokens` and `OutputTokens`. No cost calculation, no cache breakdown, no per-message tracking.
**Suggestion:** Extend `WingmanUsage`:
```go
type WingmanUsage struct {
    InputTokens   int
    OutputTokens  int
    CacheRead     int
    CacheWrite    int
    ReasoningTokens int
    Cost          float64
}
```

### 4. No Session Title Auto-Generation
**OpenCode:** Uses LLM to auto-generate a short title for each session after the first exchange.
**Wingman:** Sessions have no title field. The TUI falls back to displaying the first user message as a title.
**Suggestion:** Add a `title` field to `storage.Session` and optionally auto-generate it after the first assistant response (could be a lightweight LLM call, or just truncate the first user message).

## Important Limitations

### 5. No Undo/Redo
**OpenCode:** `POST /session/revert` and `POST /session/unrevert` allow undoing messages.
**Wingman:** No revert mechanism. Session history is append-only.
**Suggestion:** Add `POST /sessions/{id}/revert` that trims history back to a given message ID and stores the reverted messages for potential redo.

### 6. No Session Fork
**OpenCode:** `POST /session/fork` creates a new session branching from a point in the conversation.
**Wingman:** No fork endpoint.
**Suggestion:** Add `POST /sessions/{id}/fork` that copies history up to a given message and creates a new session.

### 7. No Diff/Modified Files Tracking
**OpenCode:** Tracks all files modified during a session and shows a diff sidebar with additions/deletions per file.
**Wingman:** No session-level file tracking. The TUI infers modified files from tool call inputs, but has no actual diff data.
**Suggestion:** Track file modifications in the session. Before each `write`/`edit` tool execution, snapshot the file state. Store diffs per session.

### 8. No Permission/Approval System
**OpenCode:** Has a permission system where certain tool calls (write, bash) require user approval before execution.
**Wingman:** All tools execute immediately with no approval step. The agentic loop runs autonomously.
**Suggestion:** Add a permission callback or approval channel to `Session`. When a tool requires approval, pause the loop and emit an approval-request event.

### 9. Max Steps Not Configurable
**OpenCode:** Configurable per-agent max steps.
**Wingman:** Hard-coded to 50 in `session/stream.go` and `session/session.go`.
**Suggestion:** Add `MaxSteps` to the agent config or session options.

### 10. No Working Directory Change Mid-Session
**OpenCode:** Working directory can change during a session.
**Wingman:** `Session.WorkDir` is set at creation and not updatable mid-conversation (the REST API allows `PUT /sessions/{id}` but the running session object in memory won't pick it up).
**Suggestion:** The server reconstructs the session from stored state on each message anyway, so this partially works via the REST API. But if sessions become long-lived, consider making `WorkDir` dynamic.

## Nice-to-Have Limitations

### 11. No Command Palette / Slash Commands
**OpenCode:** Rich command palette with `/` slash commands, keyboard shortcut system.
**Wingman:** No command system. All interaction is through the chat prompt.
**Suggestion:** This is a TUI concern, not necessarily a Wingman limitation. Could be implemented client-side.

### 12. No Theming
**OpenCode:** 25+ themes (catppuccin, dracula, nord, etc.) with dark/light mode.
**Wingman:** WingCode ships a single dark theme matching OpenCode's default.
**Suggestion:** TUI-level concern. Could add theme support to WingCode without Wingman changes.

### 13. No Sub-Agent / Task Delegation
**OpenCode:** Supports spawning sub-agent sessions for parallel work.
**Wingman:** Has the `actor/` package with Fleet support, but the HTTP API doesn't expose it (fleet/formation routes are commented out).
**Suggestion:** Uncomment and implement the fleet/formation routes. Add a `task` tool that spawns a sub-session.

### 14. No Session Sharing/Export
**OpenCode:** Share sessions via URL, export as markdown.
**Wingman:** No share/export endpoints.
**Suggestion:** Add `POST /sessions/{id}/export` that returns the session as formatted markdown.

### 15. No Structured Output / Markdown Rendering
**OpenCode:** Uses tree-sitter-based syntax highlighting and markdown rendering via OpenTUI's `<code>` and `<markdown>` components.
**Wingman:** WingCode renders assistant text as plain `<text>`. OpenTUI does support `<code>` and `<markdown>` components, but they require parser configuration that we haven't set up.
**Suggestion:** This can be added to WingCode with `addDefaultParsers()` from `@opentui/core` and using `<code filetype="markdown">` for assistant responses. Requires the tree-sitter WASM parsers.

## Summary

The most impactful improvements to Wingman for TUI support would be:
1. **Session abort** — essential for user control
2. **Tool result streaming** — essential for UX feedback
3. **Token/cost tracking** — important for cost-conscious users
4. **Permission system** — important for safety
5. **Session title** — improves navigation
