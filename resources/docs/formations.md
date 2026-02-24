---
title: "Formations"
group: "Wingman"
order: 6
draft: true
---

# Formations

Formations are a declarative DAG runtime for multi-agent workflows.

This document defines the proposed formation primitive in detail, including:

- How formations are represented and stored
- How they execute using actor-style inboxes
- How they run over HTTP (ephemeral runs)
- How fanout and queueing work when sections outnumber workers
- A comprehensive Deep Research example

## Why formations exist

Wingman already has strong primitives:

- `agent` for one configured worker
- `session` for iterative tool-calling conversations
- `fleet` for concurrent fanout
- `actor` for mailbox-based concurrency

Formations add a higher-level primitive: a workflow graph where each node is a unit of work and edges pass typed data between nodes. The result is predictable orchestration for non-trivial multi-agent systems.

## Design goals

- Declarative first: define workflows in YAML/JSON
- DB-canonical storage: YAML is import/export format; normalized JSON is persisted
- Actor-style execution semantics: each node processes inbox messages sequentially
- Structured dataflow: node outputs are JSON objects for stable edge mapping
- Side effects allowed: nodes may mutate files (for example `./report.md`) without requiring file state in graph payloads
- Ephemeral runs first: execute in memory, stream progress, no durable run recovery yet

## Mental model

A formation is a DAG with typed node contracts.

- Nodes receive a message in an inbox
- Each node handles one message at a time
- Node output is validated JSON
- Output is mapped onto one or more outgoing edges
- Downstream nodes receive mapped payloads in their inboxes

No shared mutable in-memory state is required for dataflow. The graph carries data by message passing. Files are side effects.

## Core concepts

### Formation

Top-level object containing metadata, node definitions, edges, and optional artifact declarations.

### Node

A workflow step.

Supported node kinds (initial set):

- `agent`: run a single agent invocation from inbox payload
- `fleet`: run a bounded worker pool against fanout items
- `join`: barrier node that waits for upstream completion

Future kinds can include `router` and `function`.

### Edge

A directed connection from one node to another with payload mapping.

### Message

A JSON object delivered to a node inbox. Node input and output are both JSON objects.

### Artifact

A file or external output modified by node side effects (for example `./report.md`). Artifacts are not required as dataflow payloads.

## Definition schema

The server accepts either YAML or JSON on create/update, validates, and stores normalized JSON.

```yaml
name: deep-research
version: 1
description: Build a technical report from a topic

defaults:
  work_dir: .

nodes:
  - id: planner
    kind: agent
    role: Planner
    agent:
      name: Planner
      provider: anthropic
      model: claude-sonnet-4-5
      instructions: |
        Create an outline with <= 8 sections (excluding Conclusion),
        create report.md skeleton, then emit sections for fanout.
      tools: [perplexity_search, webfetch, write, edit]
      output_schema:
        type: object
        required: [sections]
        properties:
          sections:
            type: array
            items:
              type: object
              required: [id, title, guidance]
              properties:
                id: { type: string }
                title: { type: string }
                guidance: { type: string }

  - id: researchers
    kind: fleet
    role: IterativeResearcher
    fleet:
      worker_count: 3
      fanout_from: planner.sections
      task_mapping:
        section_id: item.id
        section_title: item.title
        section_guidance: item.guidance
      agent:
        name: IterativeResearcher
        provider: anthropic
        model: claude-sonnet-4-5
        instructions: |
          Fill only your assigned section in report.md.
        tools: [perplexity_search, webfetch, edit]
        output_schema:
          type: object
          required: [section_id, status]
          properties:
            section_id: { type: string }
            status: { type: string, enum: [done] }

  - id: proofreader
    kind: agent
    role: Proofreader
    agent:
      name: Proofreader
      provider: anthropic
      model: claude-sonnet-4-5
      instructions: |
        Proofread and improve report.md for final quality.
      tools: [edit]
      output_schema:
        type: object
        required: [status]
        properties:
          status: { type: string, enum: [done] }

edges:
  - from: planner
    to: researchers
    map:
      sections: output.sections

  - from: researchers
    to: proofreader
    when: all_workers_done
    map:
      summary: output

artifacts:
  - name: report
    path: ./report.md
```

### Schema notes

- `nodes[].id` must be unique.
- `kind` defines runtime behavior.
- `agent.output_schema` is strongly recommended and should be required for production reliability.
- `fleet.worker_count` controls concurrency and queue behavior.
- `fanout_from` points to an upstream array output.
- `task_mapping` maps each fanout item into worker input payload fields.

## Actor execution model

Formations are executed by mapping each node to an actor with an inbox.

### Node actor semantics

- Inbox is FIFO.
- Node processes one message at a time.
- Processing emits zero or more outbound messages.
- Errors are emitted as run events and fail the run unless retry policy is configured.

### Fleet node semantics

`fleet` is a specialized actor that manages a worker pool.

- Input includes an array to fan out.
- The fleet actor enqueues one task per array item.
- A fixed number of worker actors (`worker_count`) consume tasks.
- Each worker actor processes tasks sequentially.
- If tasks > workers, remaining tasks naturally queue.

This gives the desired behavior when sections outnumber available researcher agents.

### Join semantics

`join` waits for all expected upstream completions before emitting a single downstream message.

In many flows, a fleet can emit `all_workers_done` directly without a separate join node.

## Dataflow contract

Node outputs are structured JSON and should validate against `output_schema`.

Benefits:

- Stable edge mapping (`output.sections[0].title`)
- Better runtime validation and earlier failures
- Easier API consumers and UI integrations

Side effects like writing `report.md` are allowed and intentionally separate from message payloads.

## HTTP API design

Runs are ephemeral. Definitions are persisted.

### Create formation

`POST /formations`

Accepts `application/json` or `application/x-yaml`.

**JSON request example:**

```json
{
  "name": "deep-research",
  "version": 1,
  "nodes": [
    { "id": "planner", "kind": "agent", "agent": { "name": "Planner" } }
  ],
  "edges": []
}
```

**Response example:**

```json
{
  "id": "01J...",
  "name": "deep-research",
  "version": 1,
  "created_at": "2026-02-23T12:00:00Z",
  "updated_at": "2026-02-23T12:00:00Z"
}
```

### Get/list formations

- `GET /formations`
- `GET /formations/{id}`

### Update formation

`PUT /formations/{id}` (replace definition, validate, normalize, persist)

### Export formation

`GET /formations/{id}/export?format=yaml`

Useful for round-tripping DB-canonical definitions into version-controlled YAML.

### Run formation (blocking)

`POST /formations/{id}/run`

**Request example:**

```json
{
  "inputs": {
    "topic": "State of open-source local inference in 2026"
  },
  "overrides": {
    "nodes": {
      "researchers": {
        "fleet": { "worker_count": 4 }
      }
    }
  }
}
```

**Response example:**

```json
{
  "status": "ok",
  "outputs": {
    "planner": { "sections": [{ "id": "s1", "title": "...", "guidance": "..." }] },
    "researchers": { "completed": 9 },
    "proofreader": { "status": "done" }
  },
  "artifacts": [
    { "name": "report", "path": "./report.md" }
  ],
  "stats": {
    "nodes_executed": 3,
    "duration_ms": 182340
  }
}
```

### Run formation (streaming)

`POST /formations/{id}/run/stream`

Server-Sent Events provide incremental observability.

Event types:

- `run_start`
- `node_start`
- `node_output`
- `edge_emit`
- `artifact`
- `node_end`
- `node_error`
- `run_end`

**SSE example:**

```text
event: node_start
data: {"node_id":"planner","ts":"2026-02-23T12:00:01Z"}

event: node_output
data: {"node_id":"planner","output":{"sections":[{"id":"s1","title":"Landscape","guidance":"Cover OSS and hosted"}]}}

event: edge_emit
data: {"from":"planner","to":"researchers","count":9}

event: artifact
data: {"node_id":"planner","path":"./report.md","action":"write"}

event: node_end
data: {"node_id":"planner","status":"ok"}

event: run_end
data: {"status":"ok","duration_ms":182340}
```

## Validation and failure behavior

### Validation at create/update time

- Graph must be acyclic
- All edges reference existing nodes
- `fanout_from` must target an upstream array output
- Node IDs must be unique
- Declared tools must be known by the runtime

### Validation at run time

- Required inputs present
- Node output validates against schema
- Edge mappings reference valid output fields

### Failure policy (initial)

- Default: fail-fast (first hard node error ends run)
- Optional future policy: retries per node kind with backoff

## Storage model

Formations are persisted; runs are ephemeral.

Recommended persisted fields:

- `id` (ULID)
- `name`
- `version` (int)
- `definition` (JSON text)
- `created_at`, `updated_at`

Optional future additions:

- `formation_runs` table for durable/replayable jobs
- run event log persistence for observability

## Comprehensive example: Deep Research

This example shows exactly the requested behavior:

- Planner does initial research and creates outline + report skeleton
- Sections are passed as light structured payloads to iterative workers
- Workers are fewer than sections, so tasks queue in fleet workers
- Proofreader performs final pass
- `report.md` is a shared side-effect artifact

### Full YAML

```yaml
name: deep-research
version: 1
description: Multi-agent deep research report pipeline

defaults:
  work_dir: .

inputs:
  type: object
  required: [topic]
  properties:
    topic:
      type: string

nodes:
  - id: planner
    kind: agent
    role: Planner
    agent:
      name: Planner
      provider: anthropic
      model: claude-sonnet-4-5
      instructions: |
        You are the overseer of a deep research report.
        1) Research the topic with perplexity_search, then webfetch for key sources.
        2) Produce an outline with at most 8 sections (Conclusion excluded).
        3) Create ./report.md with TOC and empty section shells.
        4) Emit JSON output containing sections with concise guidance for each researcher.
      tools: [perplexity_search, webfetch, write, edit]
      output_schema:
        type: object
        required: [sections]
        properties:
          sections:
            type: array
            minItems: 1
            items:
              type: object
              required: [id, title, guidance]
              properties:
                id:
                  type: string
                  description: Stable section key
                title:
                  type: string
                guidance:
                  type: string
                  description: Light instructions for section author

  - id: iterative_research
    kind: fleet
    role: IterativeResearcher
    fleet:
      worker_count: 3
      fanout_from: planner.sections
      task_mapping:
        section_id: item.id
        section_title: item.title
        section_guidance: item.guidance
      agent:
        name: IterativeResearcher
        provider: anthropic
        model: claude-sonnet-4-5
        instructions: |
          You are assigned exactly one report section.
          Use perplexity_search and webfetch to gather evidence.
          Edit only your assigned section in ./report.md.
          Keep style technical and concise.
        tools: [perplexity_search, webfetch, edit]
        output_schema:
          type: object
          required: [section_id, status, notes]
          properties:
            section_id:
              type: string
            status:
              type: string
              enum: [done]
            notes:
              type: string

  - id: proofreader
    kind: agent
    role: Proofreader
    agent:
      name: Proofreader
      provider: anthropic
      model: claude-sonnet-4-5
      instructions: |
        Perform final proofreading and structure cleanup for ./report.md.
        Preserve technical accuracy.
      tools: [edit]
      output_schema:
        type: object
        required: [status]
        properties:
          status:
            type: string
            enum: [done]

edges:
  - from: planner
    to: iterative_research
    map:
      sections: output.sections

  - from: iterative_research
    to: proofreader
    when: all_workers_done
    map:
      completed_sections: output.completed

artifacts:
  - name: report
    path: ./report.md
```

### How queueing works in this example

If planner emits 9 sections and `worker_count` is 3:

- Fleet creates 9 tasks
- 3 worker actors start immediately
- 6 tasks remain in queue
- As workers finish, they pull next queued task
- After all 9 tasks complete, fleet emits `all_workers_done`
- Proofreader starts

This preserves sequential inbox semantics per worker while keeping bounded concurrency overall.

### API flow for this example

1. Create formation

```bash
curl -sS -X POST http://localhost:2323/formations \
  -H "Content-Type: application/x-yaml" \
  --data-binary @resources/formations/deep-research.yml | jq .
```

2. Run streamed

```bash
curl -N -X POST http://localhost:2323/formations/<id>/run/stream \
  -H "Content-Type: application/json" \
  -d '{"inputs":{"topic":"State of open-source local inference in 2026"}}'
```

3. Inspect final artifact

```bash
cat ./report.md
```

## Implementation notes

- Start with parser + validator + executor for `agent`, `fleet`, `join`.
- Reuse existing `session` loop for `agent` nodes.
- Reuse `fleet` package semantics for `fleet` nodes.
- Back execution with `actor.System` so mailbox behavior is explicit.
- Keep runs ephemeral now; add durable run state later without changing definition schema.

## Non-goals for v1

- Durable resumable jobs
- Cross-run artifact consistency controls
- Distributed actor execution
- Advanced scheduling policies beyond FIFO queueing per fleet

## Summary

The proposed formations primitive is:

- Declarative (YAML/JSON)
- DB-canonical with YAML import/export
- Actor-driven with sequential inbox processing
- Structured in dataflow and permissive on side effects
- Strong on queueing and bounded concurrency for fanout workflows

This gives Wingman a reliable orchestration layer that scales from simple DAGs to complex multi-agent research systems.
