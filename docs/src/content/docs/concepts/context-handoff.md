---
title: "Context Handoff"
description: "How one Wingman session can move between provider and model combinations."
group: "Concepts"
draft: true
order: 201
---

# Context Handoff

Wingman sessions are not bound to one provider, one model, or even one agent. A session is the durable conversation record. The provider/model combo is selected when a message is sent.

That means the same session can start on one combo, move to another, and then move back without creating a new session:

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_fast_haiku",
    "message": "Summarize the issue."
  }'

curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_deep_sonnet",
    "message": "Use that summary to propose the safest fix."
  }'
```

Each `agent_id` points at an agent definition with its own instructions, tools, provider, model, and model options. The session keeps the transcript. The selected agent supplies the runtime configuration for that turn.

## What gets handed off

When a client sends a message to an existing session, Wingman:

1. Loads the persisted session history.
2. Loads the agent named by `agent_id`.
3. Instantiates that agent's provider/model combo.
4. Builds a fresh runtime session using the existing history and the selected agent configuration.
5. Runs the turn and appends the new messages back to the same session history.

The handoff is the transcript plus any persisted message parts. There is no hidden provider state to migrate. A different model receives the same serialized conversation history through its own provider adapter.

## Why this matters

Provider/model handoff lets clients treat model choice as a per-turn routing decision instead of a session-level commitment.

Common patterns:

- Start on a fast, cheap model for exploration.
- Escalate one hard turn to a stronger model.
- Switch providers when a model lacks a needed capability.
- Route structured-output turns through an agent configured for that schema.
- Return to the original model after the specialized turn completes.

This keeps the user's conversation continuous while still letting the client optimize for cost, latency, capability, or reliability turn by turn.

## Boundaries

Context handoff does not make every provider identical. The next model only sees the messages Wingman sends over the wire for that turn.

Watch for:

- Context window differences. A smaller model may need compaction or trimming before it can accept a long session.
- Tool differences. Switching agents can change which tools are available for the next turn.
- Instruction differences. The selected agent's instructions apply to the current turn, so a handoff can intentionally change behavior.
- Provider capability differences. Structured output, reasoning parts, image input, or tool-call formats may vary by model.

Wingman's job is to keep the session history stable and let each turn choose the best runtime configuration. The client decides when a handoff is useful.
