---
title: "Formations"
group: "Wingman"
order: 6
draft: true
---

# Formations

Formations are Wingman's declarative DAG runtime for multi-agent workflows.

The formation definition is persisted in SQLite as normalized JSON, then executed ephemerally when you call a run endpoint.

## Current implementation status

Implemented now:

- DB-canonical definitions (`name`, `version`, `definition`)
- JSON or YAML create/update payloads
- DAG validation (node/edge checks and cycle detection)
- Node kinds: `agent`, `fleet`, `join`
- Blocking run: `POST /formations/{id}/run`
- Streaming run: `POST /formations/{id}/run/stream` (SSE)
- Fleet fanout with bounded workers (`worker_count`) so queueing works when tasks > workers
- Side-effect file workflows (for example `./report.md`) via normal tools (`write`, `edit`)

Not implemented yet:

- Durable formation runs
- Retry policies
- Full JSON Schema validation of model output (the runtime currently requires parseable JSON output and requires `output_schema` to be present)
- Runtime `overrides` behavior

## Mental model

Use formations for dataflow plus side effects:

- Dataflow between nodes is structured JSON.
- File creation/editing is a side effect of tool usage in the session working directory.
- You do not need an artifact subsystem to get `report.md`; it is produced by tools.

## Definition shape

Top-level fields used by the runtime:

- `name` (required)
- `version` (optional, defaults to `1`)
- `defaults.work_dir` (optional)
- `nodes` (required)
- `edges` (optional)

### Node kinds

- `agent`: one agent run, input payload becomes the message payload for that node.
- `fleet`: fanout node; creates one task per fanout item and runs with bounded concurrency.
- `join`: barrier node that emits `{ "status": "joined" }`.

### Agent node config

`nodes[].agent`:

- `name`
- `provider`
- `model`
- `options`
- `instructions`
- `tools`
- `output_schema` (required by current validator)

### Fleet node config

`nodes[].fleet`:

- `worker_count`
- `fanout_from`
- `task_mapping`
- `agent` (same fields as agent node)

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
- `input.some_field`
- `other_node.some_field`

`edges[].when` currently supports:

- `all_workers_done` (for fleet -> downstream handoff)

## Validation rules

Create/update validation:

- `name` required
- `nodes` required
- unique node IDs
- supported node kinds only
- required node config present per kind
- edges must reference existing nodes
- graph must be acyclic

Run-time validation:

- node responses must be valid JSON

## HTTP API

Base URL: `http://localhost:2323`

### Create

`POST /formations`

Accepts JSON or YAML (`Content-Type: application/json` or `application/x-yaml`).

Response:

```json
{
  "id": "01J...",
  "name": "deep-research",
  "version": 1,
  "definition": {"...": "..."},
  "created_at": "2026-02-24T00:00:00Z",
  "updated_at": "2026-02-24T00:00:00Z"
}
```

### List/Get/Update/Delete

- `GET /formations`
- `GET /formations/{id}`
- `PUT /formations/{id}`
- `DELETE /formations/{id}`

### Export

`GET /formations/{id}/export?format=yaml`

Returns stored definition in YAML for round-tripping.

### Run (blocking)

`POST /formations/{id}/run`

Request:

```json
{
  "inputs": {
    "topic": "State of open-source local inference in 2026"
  }
}
```

Response:

```json
{
  "status": "ok",
  "outputs": {
    "planner": {"sections": [{"id": "s1", "title": "...", "guidance": "..."}]},
    "iterative_research": {"completed": 9, "all_workers_done": true, "results": []},
    "proofreader": {"status": "done"}
  },
  "stats": {
    "nodes_executed": 3,
    "duration_ms": 182340
  }
}
```

Note: `report.md` is not in this response. It is a side-effect file in the run working directory.

### Run (streaming)

`POST /formations/{id}/run/stream`

SSE event types:

- `run_start`
- `node_start`
- `node_output`
- `edge_emit`
- `node_end`
- `node_error`
- `run_end`

## Deep Research example

The repository includes a ready-to-use definition:

- `resources/formations/deep-research.yml`

Pipeline:

1. `planner` researches topic, creates `./report.md` skeleton, emits sections.
2. `iterative_research` is a fleet fanout over sections.
3. `proofreader` does final polish.

Queueing behavior:

- If planner emits more sections than `worker_count`, tasks queue automatically.
- Each worker processes one task at a time.

## Exact steps: clean DB to `report.md`

Run these from repo root.

1) Start from a clean database

```bash
rm -f /tmp/wingman-formations.db
```

2) Export required API keys (server process environment)

```bash
export ANTHROPIC_API_KEY="your-anthropic-key"
export PERPLEXITY_API_KEY="your-perplexity-key"
```

3) Start server with clean DB

```bash
go run ./cmd/wingman serve --host 127.0.0.1 --port 2323 --db /tmp/wingman-formations.db
```

4) In another terminal, configure provider auth in Wingman DB

```bash
curl -sS -X PUT http://127.0.0.1:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers":{"anthropic":{"type":"api_key","key":"'"$ANTHROPIC_API_KEY"'"}}}' | jq .
```

5) Create formation from YAML

```bash
CREATE_RESP=$(curl -sS -X POST http://127.0.0.1:2323/formations \
  -H "Content-Type: application/x-yaml" \
  --data-binary @resources/formations/deep-research.yml)

echo "$CREATE_RESP" | jq .
FORMATION_ID=$(echo "$CREATE_RESP" | jq -r '.id')
```

6) Run the formation

```bash
curl -sS -X POST "http://127.0.0.1:2323/formations/${FORMATION_ID}/run" \
  -H "Content-Type: application/json" \
  -d '{"inputs":{"topic":"State of open-source local inference in 2026"}}' | jq .
```

7) View the generated report

```bash
cat ./report.md
```

`./report.md` is written relative to the server process working directory because the example formation uses `defaults.work_dir: .`.

If the file is missing, inspect run output and server logs first; usually it means the planner node failed before writing.
