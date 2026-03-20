---
title: "Formations"
group: "Concepts"
draft: false
order: 104
---

# Formations

Formations are Wingman's declarative DAG runtime for multi-step workflows. A formation definition is persisted by the server and executed on demand.

## Runtime model

- definitions are stored in SQLite as normalized JSON
- runs are ephemeral and executed through server endpoints
- edges move structured outputs between nodes
- node kinds are `agent`, `fleet`, and `join`

Formations are currently a server feature. If you want fully in-process orchestration, compose SDK primitives directly.

## Node kinds

### `agent`

Runs one agent step. The node receives structured input and produces structured output.

### `fleet`

Fans work out across multiple tasks using the fleet runtime. This is the main way to express parallel branches inside a formation.

### `join`

Acts as a barrier node so downstream execution can wait on upstream work to complete.

## Definition shape

Top-level fields include:

- `name`
- `version`
- `defaults.work_dir`
- `nodes`
- `edges`

Agent nodes define provider, model, instructions, tools, options, and output schema. Fleet nodes define worker settings, fanout mapping, and an embedded agent configuration.

## Validation and runtime expectations

On create and update, the server validates:

- required fields
- unique node IDs
- supported node kinds
- edge references
- graph acyclicity

At run time, the current runtime expects agent and fleet-agent outputs to be parseable JSON objects, and agent-style nodes currently require `output_schema`.

## Server endpoints

```text
POST   /formations
GET    /formations
GET    /formations/{id}
PUT    /formations/{id}
DELETE /formations/{id}
GET    /formations/{id}/export
POST   /formations/{id}/run
POST   /formations/{id}/run/stream
GET    /formations/{id}/report
```

`POST /formations` accepts JSON or YAML definitions.

## Example run

```bash
curl -sS -X POST http://localhost:2323/formations/01FORM.../run \
  -H "Content-Type: application/json" \
  -d '{"inputs": {"topic": "State of local inference in 2026"}}'
```

## Streaming events

Streaming runs emit SSE events such as:

- `run_start`
- `node_start`
- `tool_call`
- `node_output`
- `edge_emit`
- `node_end`
- `node_error`
- `run_end`

## Report artifact

`GET /formations/{id}/report` reads `report.md` from the formation's configured working directory.

## Current limitations

- run history is not yet persisted for resume or replay
- retry policy configuration is not yet exposed per node or edge
- request `overrides` are accepted by the API shape but are not currently applied by the runtime
- structured output handling still expects parseable JSON objects from model responses
