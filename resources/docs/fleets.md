---
title: "Fleets"
group: "Primitives"
draft: true
order: 104
---

# Fleets

A fleet is a concurrent fan-out primitive: one agent, many tasks, bounded workers. Use it when you want to run the same agent workflow across a batch of inputs in parallel.

## SDK

```go
f := fleet.New(fleet.Config{
    Agent: a,
    Tasks: []fleet.Task{
        {Message: "Analyze auth module", WorkDir: "./src/auth", Data: "auth"},
        {Message: "Analyze API module", WorkDir: "./src/api", Data: "api"},
    },
    MaxWorkers: 2,
})

results, err := f.Run(ctx)
if err != nil {
    log.Fatal(err)
}
```

Each task can override `work_dir` and `instructions`. Results include `task_index`, `worker_name`, optional `error`, and passthrough `data`.

## Server

Fleet definitions are persisted in SQLite and reference an `agent_id`. You provide tasks at run time.

```
POST   /fleets               # Create fleet
GET    /fleets               # List fleets
GET    /fleets/{id}          # Get fleet
PUT    /fleets/{id}          # Update fleet
DELETE /fleets/{id}          # Delete fleet
POST   /fleets/{id}/run      # Run fleet (blocking)
POST   /fleets/{id}/run/stream # Run fleet (SSE)
```

### Create

```bash
curl -X POST http://localhost:2323/fleets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodebaseSweep",
    "agent_id": "01ABC...",
    "worker_count": 3,
    "work_dir": "/workspace/project"
  }'
```

### Run (blocking)

```bash
curl -X POST http://localhost:2323/fleets/01FLEET.../run \
  -H "Content-Type: application/json" \
  -d '{
    "tasks": [
      {"message": "Review auth", "work_dir": "./src/auth", "data": "auth"},
      {"message": "Review api", "work_dir": "./src/api", "data": "api"}
    ]
  }'
```

### Run (streaming)

`POST /fleets/{id}/run/stream` emits one `event: result` per completed task, then `event: done`.
