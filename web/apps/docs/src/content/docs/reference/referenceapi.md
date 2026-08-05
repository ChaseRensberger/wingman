---
title: "API"
group: "Reference"
order: 1000
---

# API

Workspace URL: `http://localhost:2323` (configurable via `--host` and `--port`).

All endpoints accept and return JSON unless noted. Non-success JSON responses
contain `error.code`, `error.message`, and `error.request_id`. The same request
ID is returned in the `X-Request-ID` header. See [HTTP API Basics](/build-clients/http-api-basics#handle-errors).

The daemon publishes an OpenAPI 3.1 document at `GET /openapi.json`.

> **Authenticated control surface:** Protected routes accept the owner credential
> or an auth session. An authenticated client can use providers, inspect
> directories, manage extensions, and run enabled tools. Wingman does not
> provide tenant isolation. See [Authentication](/concepts/authentication).

## Conventions

- Request bodies are JSON.
- Send `Authorization: Bearer <token>` on protected routes from native clients.
- Standard request timeout is 60 seconds.
- Session event endpoints and `POST /run` bypass the standard timeout; `/sessions/{id}/events` and `/run` return `text/event-stream`.
- ID prefixes are stable: `agt_` (agent), `wsp_` (Workspace), `cli_` (client), `ats_` (auth session), `ses_` (session), `msg_` (message), `prt_` (part), `tlu_` (tool use).

## Authentication endpoints

| Method | Path | Authentication | Description |
|---|---|---|---|
| `POST` | `/auth/pairings` | Owner bearer | Create a five-minute, one-use pairing credential |
| `POST` | `/auth/pairings/redeem` | Pairing credential in body | Create a cookie or bearer auth session |
| `GET` | `/auth/sessions` | Owner bearer | List auth sessions |
| `DELETE` | `/auth/sessions/{id}` | Owner bearer | Revoke an auth session |

Pairing and redemption responses use `Cache-Control: no-store`. A bearer-mode
redemption returns the session token once. A cookie-mode redemption sets an
`HttpOnly` cookie and omits the token from the response body.

## Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Authenticated readiness, instance ID, and version |

```json
{ "status": "ok" }
```

`GET /health` reports liveness and does not require authentication.
`GET /ready` requires authentication. It returns `503 Service Unavailable`
until startup recovery is complete. A non-ready response includes the failed
subsystem and a recovery action; inspect `/logs` before restarting the daemon.

```json
{
  "ready": true,
  "instance_id": "ins_...",
  "version": "0.3.0"
}
```

## Provider endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/provider` | List registered providers |
| `GET` | `/provider/{name}` | Get provider metadata |
| `GET` | `/provider/{name}/models` | List models for a provider |
| `GET` | `/provider/{name}/models/{model}` | Get model metadata |
| `GET` | `/provider/auth` | Get configured credential status |
| `PUT` | `/provider/auth` | Set credentials for one or more providers |
| `DELETE` | `/provider/auth/{provider}` | Remove credentials for a provider |
| `POST` | `/provider/{name}/oauth/authorize` | Begin browser or device OAuth authorization |
| `GET` | `/provider/{name}/oauth/{attempt}` | Read OAuth authorization status |
| `DELETE` | `/provider/{name}/oauth/{attempt}` | Cancel OAuth authorization |

### Set auth

```json
{
  "providers": {
    "anthropic": { "type": "api_key", "key": "sk-ant-..." }
  }
}
```

### Auth response

`GET /provider/auth` returns a `configured` flag per provider without leaking the secret:

```json
{
  "providers": {
    "anthropic": { "type": "api_key", "configured": true }
  },
  "updated_at": "2026-04-25T00:00:00Z"
}
```

### OpenAI Codex OAuth

Begin browser or headless authorization by posting a method:

```json
{ "method": "browser" }
```

or:

```json
{ "method": "device" }
```

`POST /provider/openai/oauth/authorize` returns `202 Accepted` with an attempt
ID, an authorization URL, and instructions. Poll the attempt URL until its
`status` is `completed`, `failed`, or `cancelled`. OAuth tokens are never
returned by these endpoints.

## Agent endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/agents` | Create agent |
| `GET` | `/agents` | List agents |
| `GET` | `/agents/{id}` | Get agent |
| `PUT` | `/agents/{id}` | Update agent (omitted fields unchanged) |
| `DELETE` | `/agents/{id}` | Delete agent |

`tools` must contain unique names from the current `GET /tools` catalog. Create
and update return `400 Bad Request` for unknown or duplicate names.

### Create request

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "tools": ["bash", "read", "write", "edit", "glob", "grep", "websearch"],
  "model_ref": "anthropic/claude-sonnet-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "output_schema": null
}
```

## Operational endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/tools` | List the unique effective native, plugin, and connected MCP catalog with input/output schemas, execution traits, source, and availability. Returns an error if sources collide. |
| `GET` | `/plugins` | List loaded external plugins and non-fatal load errors. |
| `POST` | `/plugins/reload` | Reload configured external plugins, then return plugin status. |
| `GET` | `/mcp` | List configured MCP servers and their status. |
| `POST` | `/mcp/{name}/connect` | Connect a configured MCP server. |
| `POST` | `/mcp/{name}/disconnect` | Disconnect a configured MCP server. |
| `GET` | `/clients` | List registered clients. |
| `POST` | `/clients` | Register a client by name. |
| `GET` | `/clients/{id}` | Get a registered client. |
| `GET` | `/logs` | Read up to 500 recent, process-local buffered server log entries. The buffer is cleared on restart. |
| `GET` | `/diagnostics` | Read bounded daemon state: queued and active runs, cached scopes, subscriber backlog/closure/overflow state, and aggregate plugin health. |
| `GET` | `/filesystem/directories?path=<path>` | List immediate subdirectories; omit `path` to list the server user's home directory. |

Plugin directories and MCP server definitions are configured server-wide; see [Global Config](/configure/config), [Plugins](/concepts/plugins#external-plugins), and [MCP Servers](/configure/mcp). Client records and the client header organize persisted resources only; they do not authorize requests.

`/logs` is an operational diagnostic endpoint, not durable logging or a stream. Request log entries can include paths, raw query strings, remote addresses, user agents, and client headers. Do not put secrets in API URLs, and keep the endpoint on trusted local access.

`/diagnostics` is a point-in-time operational snapshot, not durable history or
a metrics feed. Use it to identify queue buildup, disconnected event clients, or
aggregate plugin failures. Use `/plugins` for per-plugin detail and the session
and run APIs for authoritative per-run state.

## Session endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/sessions` | Create session |
| `GET` | `/sessions` | List sessions |
| `GET` | `/sessions/{id}` | Get session including history |
| `GET` | `/sessions/{id}/model-calls` | List physical upstream model attempts in start-time order |
| `GET` | `/sessions/{id}/tool-uses` | List durable tool invocations in proposal/source order |
| `GET` | `/sessions/{id}/permission-requests` | List durable permission requests in creation order |
| `GET` | `/sessions/{id}/permission-grants` | List exact remembered grants for the session |
| `POST` | `/sessions/{id}/permission-requests/{requestID}/reply` | Reply `once`, `always`, or `reject` to a pending request |
| `GET` | `/sessions/{id}/runs` | List authoritative runs in admission order |
| `GET` | `/sessions/{id}/runs/{runID}` | Get one authoritative run |
| `POST` | `/sessions/{id}/runs/{runID}/abort` | Abort one queued or locally running run |
| `POST` | `/sessions/{id}/rename` | Rename a session at an expected aggregate version |
| `POST` | `/sessions/{id}/move` | Move a session to a working directory or Workspace at an expected aggregate version |
| `DELETE` | `/sessions/{id}?expected_version={version}` | Permanently purge a session and all associated data |
| `POST` | `/sessions/{id}/message` | Durably queue a message and return its run ID (`202 Accepted`) |
| `GET` | `/sessions/{id}/events` | Replay durable events after a cursor, synchronize, then stream new events |
| `GET` | `/sessions/{id}/events/history` | Read one finite page of durable session events |
| `POST` | `/sessions/{id}/abort` | Cancel the active run; queued messages remain scheduled |
| `POST` | `/run` | Run one ephemeral session without persisting it |

Session responses include `version`, beginning at `1`. Rename and move commands
require that value as `expected_version`. A stale command returns `409 Conflict`;
reload the session before deciding whether to retry.

`POST /sessions`, `GET /sessions`, and successful rename or move commands return
session summaries. Summaries contain metadata and `version`; they do not contain
`history` or `latest_model_call`. `GET /sessions/{id}` returns the session detail
shape with both fields. An empty detail history is `[]`, not `null`.

`POST /sessions/{id}/message` requires the session to exist. Unknown IDs return `404`; message endpoints do not create sessions implicitly. Runs for one session execute in order. Queued runs survive a server restart and resume when the server starts; a run that was active at restart is recorded as aborted.

The response includes the canonical run ID, current run status, and aggregate
version after admission. Read `/sessions/{id}/runs/{runID}` for authoritative
status and `/sessions/{id}/events` for execution progress.

### Create request

```json
{
  "title": "Explore repo",
  "working_directory": "/home/me/project"
}
```

Or create the session from a Workspace:

```json
{
  "title": "Explore repo",
  "workspace_id": "wsp_..."
}
```

`working_directory` and `workspace_id` are mutually exclusive. When `workspace_id` is set, Wingman records `workspace_id` on the session and copies the Workspace path into `work_dir` if the Workspace has one.

### Rename request

```json
{
  "title": "Investigate retries",
  "expected_version": 1
}
```

### Move request

Send exactly one location field:

```json
{
  "working_directory": "/home/me/other-project",
  "expected_version": 2
}
```

Or move the session to a Workspace:

```json
{
  "workspace_id": "wsp_...",
  "expected_version": 2
}
```

Successful commands return the updated session and its new `version`. Sending
the current title or location is a no-op and leaves the version unchanged.

### Delete request

Deletion requires the current session version in `expected_version`. It returns
`409 Conflict` if the session changed after the caller read it.

```bash
curl -sS -X DELETE \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  "http://localhost:2323/sessions/ses_...?expected_version=2"
```

A successful delete permanently removes the aggregate stream, public event
history, queued and completed runs, messages, parts, model-call records, and
tool-use records.
Wingman retains no tombstone. Active SSE streams close and active execution is
canceled and settled before the response returns.

### Message request

```json
{
  "request_id": "submit-123",
  "agent_id": "agt_...",
  "message": "Write a Python script"
}
```

`request_id` is optional, opaque, scoped to this session, and limited to 200
bytes. Repeating it with the same effective input returns the existing run.
Reusing it with a different prompt, effective Agent or model, output schema,
client, or current session placement returns `409 Conflict`. Omitting it always
creates a new run. Wingman snapshots the effective Agent and placement at
admission, so later Agent edits or session moves do not redirect queued work.

### Accepted response

```json
{
  "run_id": "run_...",
  "status": "queued",
  "session_version": 4
}
```

Both a new admission and an identical retry return `202 Accepted`. On retry,
`status` is the run's current status and `session_version` is the session's
current aggregate version.

### Run response

Run statuses are `queued`, `running`, `completed`, `failed`, and `aborted`.
`GET /sessions/{id}/runs` returns an array in admission order; the single-run
endpoint returns `404` when the run does not belong to that session. Both
endpoints enforce the session's client scope.

```json
{
  "id": "run_...",
  "session_id": "ses_...",
  "request_id": "submit-123",
  "admitted_version": 4,
  "sequence": 2,
  "status": "aborted",
  "message": "Write a Python script",
  "agent": { "id": "agt_...", "name": "Builder" },
  "error_type": "process_interrupted",
  "error_message": "process interrupted during run",
  "created_at": "2026-07-30T12:00:00Z",
  "started_at": "2026-07-30T12:00:01Z",
  "completed_at": "2026-07-30T12:00:03Z",
  "updated_at": "2026-07-30T12:00:03Z"
}
```

On startup, running runs are recorded as aborted. Unfinished tool uses are
interrupted, partial messages are retained as failed, and queued runs resume.
Wingman does not replay provider calls or tool side effects that may already
have run.

### Model-call response

`GET /sessions/{id}/model-calls` returns one record per physical upstream
attempt. Durable attempts include `run_id`; all calls include stable `id`,
`step`, `attempt`, `status`, route, timing, usage, and error fields. A
`provider_request_id` is included when the provider returns a supported request
ID header. `assistant_message_id` appears when the attempt produced a persisted
assistant message.

```json
[
  {
    "id": "mcl_...",
    "session_id": "ses_...",
    "run_id": "run_...",
    "assistant_message_id": "msg_...",
    "step": 1,
    "attempt": 1,
    "status": "completed",
    "model_ref": "anthropic/claude-sonnet-5",
    "provider_request_id": "req_...",
    "input_tokens": 120,
    "output_tokens": 48,
    "total_tokens": 168,
    "context_tokens": 168,
    "started_at": "2026-07-30T12:00:00Z",
    "completed_at": "2026-07-30T12:00:02Z"
  }
]
```

### Tool-use response

`GET /sessions/{id}/tool-uses` returns the authoritative lifecycle for every
model-proposed tool invocation. Rows are ordered by proposal time and source
ordinal. `id` is Wingman's stable identity; `call_id` is provider correlation
data and may repeat across runs.

```json
[
  {
    "id": "tlu_...",
    "session_id": "ses_...",
    "run_id": "run_...",
    "model_call_id": "mcl_...",
    "assistant_message_id": "msg_...",
    "part_id": "prt_...",
    "step": 1,
    "ordinal": 1,
    "call_id": "call_...",
    "name": "bash",
    "status": "completed",
    "input": { "command": "pwd" },
    "output": "/home/me/project",
    "metadata": { "exit_code": 0 },
    "proposed_at": "2026-07-30T12:00:02Z",
    "authorized_at": "2026-07-30T12:00:02Z",
    "started_at": "2026-07-30T12:00:02Z",
    "completed_at": "2026-07-30T12:00:03Z"
  }
]
```

Statuses are `proposed`, `authorized`, `started`, `completed`, `failed`,
`interrupted`, or `declined`. On server startup, unfinished records become
`interrupted`; Wingman does not automatically replay them.

### Permission requests

An authored `ask` rule creates a pending request after tool proposal and input
validation but before tool authorization. The tool remains suspended until a
reply, timeout, run cancellation, or shutdown recovery resolves it.

```json
{
  "id": "prq_...",
  "session_id": "ses_...",
  "run_id": "run_...",
  "tool_use_id": "tlu_...",
  "call_id": "call_...",
  "action": "edit",
  "resources": ["/home/me/project/src/main.go"],
  "status": "pending",
  "created_at": "2026-07-30T12:00:00Z",
  "updated_at": "2026-07-30T12:00:00Z"
}
```

Reply with:

```json
{ "response": "once" }
```

`once` and `always` resolve the request as `approved`; `reject` resolves it as
`rejected`. `always` also atomically stores exact session-scoped grants for the
request's action/resources. Identical reply retries return `200` with the
existing request and no duplicate event. A conflicting terminal reply returns
`409`; unknown requests return `404`.

Other terminal statuses are `timed_out` and `interrupted`. Pending requests use
a five-minute server timeout and become interrupted when their run is canceled
or during startup recovery. Durable events are `session.permission.requested`
and `session.permission.resolved`.

### Streaming

`GET /sessions/{id}/events?after=<seq>` returns `text/event-stream`. `after` is
an exclusive durable cursor. When `after` is absent, Wingman reads the
`Last-Event-ID` header. The query parameter takes precedence when both are
present.

The server captures a durable watermark, replays every sequence through it in
pages, emits `session.events.synchronized`, and then delivers live events. The
`limit` parameter controls replay page size, not total replay length; its default
is `100` and maximum is `500`. If delivery overflows or a cursor cannot be
reconciled, the server emits `session.events.resync_required` and disconnects.
Reload authoritative session and run state, then reconnect from the last durable
cursor.

Each event is:

```text
event: <type>
data: <json>

```

Where `<json>` is the event envelope:

```json
{ "id": "evt_...", "type": "session.message.created", "cursor": {"session_id":"ses_...","seq":12}, "data": { ... } }
```

See [Streaming Events](/build-clients/streaming-events) for event shapes and reconnect behavior.

### Abort response

```json
{ "session_id": "ses_...", "aborted": 1 }
```

`aborted` is `1` when the active run was asked to cancel and `0` when no run is
active. Queued runs remain scheduled. The session-level endpoint is idempotent.
To abort a specific run, use `POST /sessions/{id}/runs/{runID}/abort`: queued
runs settle immediately and return `200`; a locally running run is signaled and
returns `202`; terminal runs or a running run not owned by this server return
`409`.

## Workspace endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/workspaces` | Create Workspace |
| `GET` | `/workspaces` | List Workspaces for the active client |
| `GET` | `/workspaces/{id}` | Get Workspace |
| `PUT` | `/workspaces/{id}` | Update Workspace metadata (name, optional path) |
| `DELETE` | `/workspaces/{id}` | Delete Workspace |
| `GET` | `/workspaces/{id}/sessions` | List sessions in a Workspace |

### Create Workspace request

```json
{
  "name": "Wingman",
  "path": "/home/me/project"
}
```

Use an empty `path` for a Workspace that should not provide a working directory.

Workspaces are scoped by `X-Wingman-Client`. Omitting the header uses the built-in `Wingman` client (`cli_wingman`).

## Ephemeral run endpoint

`POST /run` creates an in-memory session, streams the run, and does not persist
the session or its messages. Unlike persistent session SSE, it uses the one-shot
run event vocabulary (including `stream_part`) and ends with `done` on success
or `error` on a terminal failure; it cannot be replayed.

In normal persistent mode, pass either `agent_id` or an inline `agent`:

```json
{
  "agent_id": "agt_...",
  "message": "Summarize this project",
  "working_directory": "/home/me/project"
}
```

When the server is started with `--ephemeral`, persisted agents are unavailable, so pass an inline agent:

```json
{
  "agent": {
    "name": "One-shot Assistant",
    "instructions": "Be concise.",
    "tools": ["webfetch", "websearch"],
    "model_ref": "anthropic/claude-sonnet-5"
  },
  "message": "Explain Wingman in one paragraph."
}
```
