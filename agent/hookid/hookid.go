// Package hookid provides stable string identifiers for the hooks defined in
// agent/run.
//
// These IDs are used for wire vocabulary, observability span/trace event
// names, and documentation anchors. They are explicitly NOT used for in-process
// dispatch — that is handled by the Go types in agent/run directly.
package hookid

// ID is a stable string identifier for a hook. Use the named constant values
// rather than raw strings so callers get compile-time safety.
type ID string

const (
	RunBefore         ID = "run.before"
	RunAfter          ID = "run.after"
	TurnStart         ID = "turn.start"
	TurnEnd           ID = "turn.end"
	HistoryTransform  ID = "history.transform"
	SystemTransform   ID = "system.transform"
	ContextTransform  ID = "context.transform"
	ToolDefsTransform ID = "tooldefs.transform"
	ParamsTransform   ID = "params.transform"
	ToolBefore        ID = "tool.before"
	ToolAfter         ID = "tool.after"
	EventSink         ID = "event.sink"
)

type Hook struct {
	ID          ID     // stable string identifier, e.g. "tool.before"
	GoSymbol    string // dotted path to the Go symbol, e.g. "Hooks.BeforeToolCall" or "Sink.OnEvent"
	Description string
}

var hooks = []Hook{
	{
		ID:          RunBefore,
		GoSymbol:    "Hooks.BeforeRun",
		Description: "Fires exactly once at the start of Run, after validation and before the first iteration.",
	},
	{
		ID:          RunAfter,
		GoSymbol:    "Hooks.AfterRun",
		Description: "Fires exactly once at the end of Run, after termination for any reason.",
	},
	{
		ID:          TurnStart,
		GoSymbol:    "Hooks.OnTurnStart",
		Description: "Fires at the top of each turn, after MaxSteps check and after TransformHistory; observation only.",
	},
	{
		ID:          TurnEnd,
		GoSymbol:    "Hooks.OnTurnEnd",
		Description: "Fires after a turn's assistant message and tool results have been appended; observation only.",
	},
	{
		ID:          HistoryTransform,
		GoSymbol:    "Hooks.TransformHistory",
		Description: "Fires at the top of each loop iteration, before OnTurnStart; may persist mutations into running history (e.g. compaction, budget enforcement).",
	},
	{
		ID:          SystemTransform,
		GoSymbol:    "Hooks.TransformSystem",
		Description: "May rewrite the system prompt for this turn only.",
	},
	{
		ID:          ContextTransform,
		GoSymbol:    "Hooks.TransformContext",
		Description: "May rewrite the message history for this turn only; mutations are ephemeral.",
	},
	{
		ID:          ToolDefsTransform,
		GoSymbol:    "Hooks.TransformToolDefs",
		Description: "May rewrite the tool definitions for this turn's wire request only.",
	},
	{
		ID:          ParamsTransform,
		GoSymbol:    "Hooks.TransformParams",
		Description: "May rewrite the sampling parameters for this turn's wire request only.",
	},
	{
		ID:          ToolBefore,
		GoSymbol:    "Hooks.BeforeToolCall",
		Description: "Fires for each tool call before execution; may rewrite args or skip the call.",
	},
	{
		ID:          ToolAfter,
		GoSymbol:    "Hooks.AfterToolCall",
		Description: "Fires after each tool call's execution; may rewrite the result string.",
	},
	{
		ID:          EventSink,
		GoSymbol:    "Sink.OnEvent",
		Description: "Receives loop lifecycle events via the event sink interface.",
	},
}

// All returns a copy of the registry slice in registration order.
func All() []Hook {
	out := make([]Hook, len(hooks))
	copy(out, hooks)
	return out
}

// IDs returns the stable identifiers of all registered hooks in registration order.
func IDs() []ID {
	out := make([]ID, len(hooks))
	for i, h := range hooks {
		out[i] = h.ID
	}
	return out
}

// Lookup returns the Hook associated with id, or the zero value and false if
// id is not registered.
func Lookup(id ID) (Hook, bool) {
	for _, h := range hooks {
		if h.ID == id {
			return h, true
		}
	}
	return Hook{}, false
}
