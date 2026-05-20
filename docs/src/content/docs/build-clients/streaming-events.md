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

## When To Stream

Stream when your client needs to:

- Render assistant output as it arrives.
- Show tool calls in progress.
- Display lifecycle events.
- Let users cancel long-running work.

Use the blocking message endpoint when you only need the final response.
