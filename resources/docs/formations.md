---
title: "Formations"
group: "Primitives"
draft: true
order: 105
---

# Formations

Formations are Wingman's declarative DAG runtime for multi-agent workflows.

The definition is persisted in SQLite as normalized JSON, then executed ephemerally when you call a run endpoint.

## Runtime model

- Nodes define units of work: `agent`, `fleet`, `join`.
- Edges map upstream outputs into downstream node inputs.
- Dataflow between nodes is structured JSON (`map[string]any`).
- Runs are ephemeral; persisted state is the formation definition, not run history.
- Tool side effects (for example writing `report.md`) happen in the configured `work_dir`.

## Current scope

Today, formations execute through the HTTP server runtime. For fully in-process orchestration, use SDK primitives directly (`agent`, `session`, `fleet`).

## Definition shape

Top-level fields used by the runtime:

- `name` (required)
- `version` (optional, defaults to `1`)
- `defaults.work_dir` (optional, defaults to `.`)
- `nodes` (required)
- `edges` (optional)

### Node kinds

- `agent`: one agent run. The node input payload becomes the message payload (JSON string by default, or `input.message` when present).
- `fleet`: fan-out node. Creates one task per fanout item and runs workers with bounded concurrency.
- `join`: barrier node that emits `{ "status": "joined" }`.

### Agent node config

`nodes[].agent`:

- `name`
- `provider` (required)
- `model` (required)
- `options`
- `instructions`
- `tools`
- `output_schema` (required by current validator)

### Fleet node config

`nodes[].fleet`:

- `worker_count`
- `fanout_from` (required)
- `task_mapping`
- `agent` (same fields as `nodes[].agent`; `provider`, `model`, and `output_schema` are required)

`fanout_from` supports:

- `input.some_field`
- `node_id.some_field`

`task_mapping` expressions support:

- `item`
- `item.some_field`
- literal string fallback

## Edge mapping and conditions

`edges[].map` maps downstream payload keys from expressions:

- `output`
- `output.some_field`
- `input`
- `input.some_field`
- `other_node.some_field`

`edges[].when` currently supports:

- `all_workers_done` (for fleet -> downstream handoff)

## Validation rules

Create/update validation:

- `name` required
- `nodes` required
- unique node IDs
- supported node kinds only (`agent`, `fleet`, `join`)
- required node config present per kind
- edges must reference existing nodes
- graph must be acyclic

Run-time expectations:

- agent/fleet model responses must be parseable JSON objects
- `output_schema` must be set on agent/fleet-agent configs

## Server

```
POST   /formations                 # Create formation (JSON or YAML)
GET    /formations                 # List formations
GET    /formations/{id}            # Get formation
PUT    /formations/{id}            # Update formation
DELETE /formations/{id}            # Delete formation
GET    /formations/{id}/export     # Export definition (json|yaml)
POST   /formations/{id}/run        # Run formation (blocking)
POST   /formations/{id}/run/stream # Run formation (SSE)
GET    /formations/{id}/report     # Read report.md from formation work_dir
```

### Create

`POST /formations` accepts JSON or YAML (`application/json` or `application/x-yaml`).

```bash
curl -X POST http://localhost:2323/formations \
  -H "Content-Type: application/json" \
  -d '{
    "name": "deep-research",
    "version": 1,
    "defaults": {"work_dir": "."},
    "nodes": [
      {
        "id": "planner",
        "kind": "agent",
        "agent": {
          "name": "Planner",
          "provider": "anthropic",
          "model": "claude-sonnet-4-5",
          "instructions": "Plan a report and emit JSON",
          "output_schema": {"type": "object"}
        }
      }
    ]
  }'
```

### Run (blocking)

```bash
curl -X POST http://localhost:2323/formations/01FORM.../run \
  -H "Content-Type: application/json" \
  -d '{"inputs": {"topic": "State of local inference in 2026"}}'
```

Response shape:

```json
{
  "status": "ok",
  "outputs": {
    "planner": {"sections": []},
    "iterative_research": {"completed": 0, "all_workers_done": true, "results": []}
  },
  "stats": {
    "nodes_executed": 2,
    "duration_ms": 1234
  }
}
```

### Run (streaming)

`POST /formations/{id}/run/stream` emits SSE events such as:

- `run_start`
- `node_start`
- `tool_call`
- `node_output`
- `edge_emit`
- `node_end`
- `node_error`
- `run_end`

### Report artifact

`GET /formations/{id}/report` reads `report.md` from `defaults.work_dir` (or `.` if omitted).

## Deep research reference

The repository includes a ready-to-use definition at `resources/formations/deep-research.yml`.

Pipeline:

1. `planner` plans and writes `report.md`.
2. `iterative_research` is a fleet fanout over sections.
3. `proofreader` does final polish.

Queueing behavior:

- If fanout items exceed `worker_count`, tasks queue automatically.
- `worker_count <= 0` means effectively unbounded parallelism (up to item count).

## Current limitations

- No durable formation runs yet (run state is not persisted for resume/replay).
- No retry policy configuration per node/edge.
- `run` request `overrides` are accepted by API shape but currently not applied by runtime.
- Runtime requires parseable JSON object output; full schema-level output validation is still limited.
- There is runtime-specific behavior for a node named `planner` that enforces writing non-empty `report.md`.
