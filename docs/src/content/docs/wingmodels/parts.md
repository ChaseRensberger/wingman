---
title: "Parts"
group: "Concepts"
draft: false
order: 103
---

# Parts

A `wingmodels.Message` is a role plus a list of `Part` values. `Part` is a discriminated union: each implementation reports a stable string discriminator via `Type()`, and the JSON serialization carries that discriminator as a `"type"` field.

## Built-in parts

The core types live in `wingmodels`:

| Discriminator | Type | Carries |
|---|---|---|
| `text` | `TextPart` | Plain assistant or user text |
| `reasoning` | `ReasoningPart` | Reasoning / chain-of-thought (when the provider exposes it) |
| `image` | `ImagePart` | Inline image data + mime type |
| `tool_call` | `ToolCallPart` | Model-emitted request to invoke a tool |
| `tool_result` | `ToolResultPart` | Outcome of executing a tool call |

The `Part` interface is sealed to the `wingmodels` package via an unexported marker, so no foreign Go type can implement it directly. The discriminator name `reasoning` matches Vercel AI SDK v3 and generalizes across Anthropic extended thinking, OpenAI o1/o3, and DeepSeek R1.

## Plugin parts via `OpaquePart`

Plugins extend the union without breaking the seal. They register a discriminator string and a decoder, and serialize their payloads as `OpaquePart`:

```go
type OpaquePart struct {
    TypeName string
    Raw      json.RawMessage
}
```

`OpaquePart` satisfies `Part` (its `isPart` marker lives in `wingmodels`) and round-trips its raw bytes verbatim. Plugins ship a typed accessor so consumers don't deal with raw JSON. The compaction plugin's `MarkerPart` is the canonical example:

```go
import "github.com/chaserensberger/wingman/plugins/compaction"

for _, p := range msg.Content {
    if marker, ok := compaction.DecodeMarker(p); ok {
        fmt.Printf("compacted %d messages: %s\n",
            marker.OriginalCount, marker.Summary)
    }
}
```

## Open registry

`wingmodels.RegisterPart(typeName, decoder)` registers a discriminator with the global decoder registry. `UnmarshalPart` dispatches on `"type"`:

- Known discriminator → call the registered decoder.
- Unknown discriminator → return an `OpaquePart` preserving the original bytes.

This is what makes plugin removal safe: a session that contains a plugin's custom parts still loads after the plugin is uninstalled. The parts come back as opaque values; UIs can render a placeholder or skip them, and re-marshaling still produces the original payload.

Plugins typically register from `Install`:

```go
func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterPart("my_marker", func(data []byte) (wingmodels.Part, error) {
        raw := make([]byte, len(data))
        copy(raw, data)
        return wingmodels.OpaquePart{TypeName: "my_marker", Raw: raw}, nil
    })
    // ... hooks, tools, sinks
    return nil
}
```

`RegisterPart` is idempotent across re-installs and may be called from `init()`.

## Wire format

`MarshalPart` writes `{"type": "...", ...body}` where `body` is the part's struct serialization. `OpaquePart` shortcuts to its stored raw bytes so unknown plugin parts round-trip exactly. `ToolResultPart`'s nested `Output` array is hand-rolled so each child also carries its discriminator.

The same encoding is used in storage (per-part rows include the `"type"` field) and on the SSE wire (where parts ride inside `MessageEvent` and `FinishPart` payloads).
