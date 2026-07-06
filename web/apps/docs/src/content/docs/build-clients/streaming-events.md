---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Use the streaming endpoint when a client needs live updates while a session runs.

```bash
curl -N -X POST "http://localhost:2323/sessions/${SESSION_ID}/message/stream" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "agt_...",
    "message": "Summarize this project."
  }'
```

Wingman sends server-sent events:

```text
event: <type>
data: <json>
```

The `data` payload is an envelope:

```json
{
  "type": "tool_start",
  "version": 1,
  "data": {}
}
```

The stream ends with a terminal `done` event containing usage and step count.

Common event types:

| Event | Use |
|---|---|
| `stream_part` | Append assistant output or model stream metadata. |
| `tool_start` | Show that a tool call started. |
| `tool_end` | Mark a tool call complete and render a short result. |
| `message` | Reconcile persisted assistant history when needed. |
| `error` | Show an error and unlock the client UI. |
| `done` | Mark the run complete and record usage/step count. |

At minimum, a client should parse `data:` lines, JSON-decode the envelope, handle `stream_part`, `error`, and `done`, and ignore unknown event types for forward compatibility.

## When To Stream

Stream when your client needs to:

- Render assistant output as it arrives.
- Show tool calls in progress.
- Display lifecycle events.
- Let users cancel long-running work.

Use the blocking message endpoint when you only need the final response.

For the complete streaming shape, see [API](/docs/reference/referenceapi#streaming).
