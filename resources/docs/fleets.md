---
title: "Fleets"
group: "Concepts"
draft: false
order: 103
---

# Fleets

Fleets are Wingman's high-level concurrency primitive. A fleet takes one agent template and runs it across many tasks in parallel.

Use a fleet when you want to fan one workflow out over a batch of inputs such as directories, documents, repositories, or research topics.

## Task model

Each task can supply:

- `message` - required prompt for that worker
- `work_dir` - optional working directory override
- `instructions` - optional system prompt override
- `data` - arbitrary passthrough metadata returned with the result

If `instructions` is set, Wingman creates an agent copy for that task that shares the provider, tools, and schema while swapping only the instructions.

## Concurrency

`MaxWorkers` bounds concurrent execution.

- `0` means unbounded concurrency
- positive values limit the number of active workers
- excess tasks queue until a worker is free

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

## Streaming

Fleet execution is also available in streaming mode so results can be processed as workers finish.

```go
fs, err := f.RunStream(ctx)
if err != nil {
    log.Fatal(err)
}

for fs.Next() {
    r := fs.Result()
    fmt.Printf("worker %s done: %s\n", r.WorkerName, r.Result.Response)
}
```

## Server

Fleet definitions are persisted in SQLite and reference an `agent_id`. Tasks are provided when the fleet is run.

```text
POST   /fleets
GET    /fleets
GET    /fleets/{id}
PUT    /fleets/{id}
DELETE /fleets/{id}
POST   /fleets/{id}/run
POST   /fleets/{id}/run/stream
```

### Create a fleet

```bash
curl -sS -X POST http://localhost:2323/fleets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodebaseSweep",
    "agent_id": "01ABC...",
    "worker_count": 3,
    "work_dir": "/workspace/project"
  }'
```

### Run a fleet

```bash
curl -sS -X POST http://localhost:2323/fleets/01FLEET.../run \
  -H "Content-Type: application/json" \
  -d '{
    "tasks": [
      {"message": "Review auth", "work_dir": "./src/auth", "data": "auth"},
      {"message": "Review api", "work_dir": "./src/api", "data": "api"}
    ]
  }'
```

`POST /fleets/{id}/run/stream` emits one `result` event per completed worker followed by `done`.
