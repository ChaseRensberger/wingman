---
title: "SKILL"
group: "Reference"
order: 1001
---
# Wingman Skill

> Disclaimer: You can copy the contents of this file into your coding agent's skill directory to make building with Wingman as easy as possible.

---

---
name: wingman
description: Use when building with Wingman, a self-hostable Go agent orchestration engine with SDK and HTTP server modes, provider/model separation, tools, sessions, and fleets.
license: MIT
metadata:
  author: https://github.com/chaserensberger
  version: "0.5.1"
  triggers: Wingman, wingman, agent orchestration, Go SDK agents, self-hosted agents, airgapped AI, session tool use, fleet fanout, formations
  role: specialist
  scope: implementation
  output-format: code
---

This skill gives an AI coding agent the minimum context needed to build with Wingman correctly.

## What Wingman is

Wingman is a self-hostable, airgap-friendly agent orchestration engine in Go with two interchangeable usage modes:

1. Go SDK (in-process, caller controls persistence)
2. HTTP server (`wingman serve`, persisted in SQLite)

Core design goals:

- Self-contained runtime (no external provider/model registries required)
- Works in airgapped environments
- Provider + model treated as separate first-class fields
- Composable abstractions: agent -> session -> fleet -> formations

## Repository map

Use these packages as canonical boundaries:

- `core/`: shared types + interfaces
- `agent/`: agent config and options
- `session/`: agentic loop (blocking + streaming)
- `fleet/`: concurrent fan-out execution
- `actor/`: lower-level actor primitives (advanced/legacy)
- `provider/`: provider registry + metadata
- `provider/anthropic/`, `provider/ollama/`: provider implementations
- `tool/`: tool interfaces + built-in tools
- `internal/server/`: HTTP API handlers
- `internal/storage/`: SQLite storage

## Non-negotiable modeling rules

1. Keep `provider` and `model` as separate fields.
2. Keep model/options as `map[string]any` (`Options`) and pass through to providers.
3. Import shared types/interfaces from `core`, not duplicated local structs.
4. Provider packages must implement `core.Provider` and register via `provider.Register(...)`.

## Agent mental model

An `agent.Agent` is a named configuration bundle:

- instructions (system prompt)
- provider (`core.Provider`)
- tools (`[]core.Tool`)
- optional output JSON schema

Create agents with functional options (`agent.WithInstructions`, `agent.WithProvider`, `agent.WithTools`, etc.).

## Session mental model

A session is conversation state + agent execution context (`work_dir`, message history).

Agentic loop behavior:

1. Append user message
2. Build `core.InferenceRequest`
3. Run provider inference
4. Append assistant response
5. If `stop_reason == "tool_use"`, execute tool calls, append `tool_result`, and continue
6. Return final result

Important:

- SDK sessions are in-memory.
- Server sessions are persisted and reconstructed per request.
- Tool call ID pairing matters: `tool_result.tool_use_id` must match the prior `tool_use.id`.

## Fleet mental model

`fleet.Fleet` is the primary high-level fan-out primitive:

- one template agent
- many tasks
- optional per-task overrides (`message`, `work_dir`, `instructions`, `data`)
- bounded concurrency via `MaxWorkers` (`0` means unlimited)

Use `Run` for blocking all results, `RunStream` for incremental results.

## Formations mental model

Formations are an implemented server-side DAG runtime for multi-agent workflows.

- Definitions are persisted (`formations` table) and runs are ephemeral.
- Node kinds: `agent`, `fleet`, `join`.
- Edges map structured outputs into downstream inputs (`edges[].map`) with optional `when: all_workers_done`.
- `fleet` nodes fan out from an array path (`fanout_from`) with bounded concurrency via `worker_count`.
- Runtime requires agent/fleet-agent `output_schema` and parseable JSON object outputs.

Current `actor/` status:

- `actor/` provides lower-level mailbox primitives (`Actor`, `System`, `Ref`, `Message`).
- `actor.Fleet` exists for compatibility; `fleet.Fleet` is the recommended fan-out API.
- Formation execution currently uses the dedicated runtime in `internal/server/formations_runtime.go`.

## Providers

Provider construction options are intentionally provider-specific.

- Factory path: `provider.New(providerID, opts)`
- Direct path: provider constructor (for example, `anthropic.New(...)`)

Known option keys:

- Anthropic: `model`, `max_tokens`, `temperature`, `api_key`
- Ollama: `model`, `max_tokens` (mapped), `temperature`, `base_url`

Operational notes:

- Anthropic requires `max_tokens`; provider defaults it to `4096` if absent.
- Server auth keys come from Wingman SQLite auth storage, not process env vars.
- SDK path may use env var fallback (for example `ANTHROPIC_API_KEY`).

## Built-in tools

Built-in tool names:

- `bash`
- `read`
- `write`
- `edit`
- `glob`
- `grep`
- `webfetch`

SDK supports custom tools via `core.Tool`; server resolves built-ins by name only.

## HTTP API quick map

- Health: `GET /health`
- Provider metadata/models/auth: `/provider...`
- CRUD agents: `/agents`
- CRUD sessions + run message + stream: `/sessions`
- CRUD fleets + run + stream: `/fleets`
- CRUD formations + export + run + stream + report: `/formations`

Conventions:

- JSON request/response bodies
- Errors as `{ "error": "..." }`
- Streaming endpoints use SSE and `StreamEvent`-style payloads

## Persistence defaults

- SQLite path: `~/.local/share/wingman/wingman.db`
- Primary tables: `agents`, `sessions`, `fleets`, `formations`, `auth`

## Recommended implementation workflow for agents

When adding features:

1. Update `core` first if new shared types/interfaces are needed.
2. Keep provider integration behind `core.Provider` and registry factory.
3. Preserve server/SDK parity (same concepts, different runtime wiring).
4. Add or update examples under `examples/`.
5. Document behavior changes in `resources/docs/master.md`.

When debugging runtime issues:

1. Verify provider/model/options wiring.
2. Verify tool definitions are included in inference request.
3. Verify tool call ID and tool result ID matching.
4. Verify session history replay/persistence behavior (server mode).
5. Verify streaming events and stop reason handling.

## Known current limitations

- No built-in provider capability database (context windows/features are user-known).
- Server does not load user-defined custom tools.
- Session loop has no configurable max-steps ceiling yet.
- Formation runs are ephemeral (no durable run records/resume), and request `overrides` are not applied yet.

## Minimal SDK pattern (reference)

```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})
if err != nil { /* handle */ }

a := agent.New("Coder",
    agent.WithInstructions("You are a senior Go developer."),
    agent.WithProvider(p),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool(), tool.NewWriteTool()),
)

s := session.New(session.WithAgent(a))
result, err := s.Run(ctx, "Write hello.go and run it")
```

Use this file as the portable, agent-facing quick reference for Wingman.
