---
title: "Formations"
group: "Primitives"
order: 105
---

# Formations

Formations are declarative multi-agent workflows represented as a DAG (directed acyclic graph). They are stored as definitions and executed on demand.

## Runtime model

- Nodes define units of work (`agent`, `fleet`, `join`).
- Edges route structured outputs to downstream node inputs.
- Runs are ephemeral; the definition is persisted, not the run state.
- Tool side effects (for example writing `report.md`) happen in the configured work directory.

## Current scope

Today, formations execute through the HTTP server runtime. The SDK primitives (`agent`, `session`, `fleet`) are still the best path for fully in-process orchestration.

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

### Run (streaming)

`POST /formations/{id}/run/stream` emits SSE events such as `run_start`, `node_start`, `node_output`, `node_end`, `node_error`, and `run_end`.
