---
title: "API"
group: "Reference"
order: 1000
---

# API

Workspace URL: `http://localhost:2323` (configurable via `--host` and `--port`).

All endpoints accept and return JSON unless noted. Error responses use the shape `{"error": "..."}`.

> **Trusted-local control surface:** Wingman has no inbound authentication or tenant isolation. A caller that can reach the server can use its configured providers, inspect local directories, manage extensions, and start agents that may invoke enabled local tools. Keep it bound to trusted local access; `X-Wingman-Client` is attribution, not an access boundary. See [Global Config](/configure/config) and [Run the Server](/use-wingman/run-server).

## Conventions

- Request bodies are JSON.
- Standard request timeout is 60 seconds.
- Session event endpoints and `POST /run` bypass the standard timeout; `/sessions/{id}/events` and `/run` return `text/event-stream`.
- ID prefixes are stable: `agt_` (agent), `wsp_` (Workspace), `cli_` (client), `ses_` (session), `msg_` (message), `prt_` (part), `tlu_` (tool use).

## Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |

```json
{ "status": "ok" }
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
| `POST` | `/provider/openai/oauth/authorize` | Begin browser or device OAuth authorization |
| `GET` | `/provider/openai/oauth/{attempt}` | Read OAuth authorization status |
| `DELETE` | `/provider/openai/oauth/{attempt}` | Cancel OAuth authorization |

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
| `GET` | `/tools` | List native, plugin, and MCP tools with their advertised input schemas and availability. |
| `GET` | `/plugins` | List loaded external plugins and non-fatal load errors. |
| `POST` | `/plugins/reload` | Reload configured external plugins, then return plugin status. |
| `GET` | `/mcp` | List configured MCP servers and their status. |
| `POST` | `/mcp/{name}/connect` | Connect a configured MCP server. |
| `POST` | `/mcp/{name}/disconnect` | Disconnect a configured MCP server. |
| `GET` | `/clients` | List registered clients. |
| `POST` | `/clients` | Register a client by name. |
| `GET` | `/clients/{id}` | Get a registered client. |
| `GET` | `/logs` | Read up to 500 recent, process-local buffered server log entries. The buffer is cleared on restart. |
| `GET` | `/filesystem/directories?path=<path>` | List immediate subdirectories; omit `path` to list the server user's home directory. |

Plugin directories and MCP server definitions are configured server-wide; see [Global Config](/configure/config), [Plugins](/concepts/plugins#external-plugins), and [MCP Servers](/configure/mcp). Client records and the client header organize persisted resources only; they do not authorize requests.

`/logs` is an operational diagnostic endpoint, not durable logging or a stream. Request log entries can include paths, raw query strings, remote addresses, user agents, and client headers. Do not put secrets in API URLs, and keep the endpoint on trusted local access.

## Session endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/sessions` | Create session |
| `GET` | `/sessions` | List sessions |
| `GET` | `/sessions/{id}` | Get session including history |
| `GET` | `/sessions/{id}/model-calls` | List recorded upstream model calls for the session |
| `POST` | `/sessions/{id}/rename` | Rename a session at an expected aggregate version |
| `POST` | `/sessions/{id}/move` | Move a session to a working directory or Workspace at an expected aggregate version |
| `DELETE` | `/sessions/{id}?expected_version={version}` | Permanently purge a session and all associated data |
| `POST` | `/sessions/{id}/message` | Durably queue a message and return its run ID (`202 Accepted`) |
| `GET` | `/sessions/{id}/events` | Replay one bounded page of durable events after `after`, then stream new events |
| `GET` | `/sessions/{id}/events/history` | Read one finite page of durable session events |
| `POST` | `/sessions/{id}/abort` | Cancel the active run; queued messages remain scheduled |
| `POST` | `/run` | Run one ephemeral session without persisting it |

Session responses include `version`, beginning at `1`. Rename and move commands
require that value as `expected_version`. A stale command returns `409 Conflict`;
reload the session before deciding whether to retry.

`POST /sessions/{id}/message` requires the session to exist. Unknown IDs return `404`; message endpoints do not create sessions implicitly. Runs for one session execute in order. Queued runs survive a server restart and resume when the server starts; a run that was active at restart is recorded as aborted.

The response includes the canonical run ID, current run status, and aggregate
version after admission. Read `/sessions/{id}/events` for execution progress and
the terminal result.

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
  "http://localhost:2323/sessions/ses_...?expected_version=2"
```

A successful delete permanently removes the aggregate stream, public event
history, queued and completed runs, messages, parts, and model-call records.
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

### Streaming

`GET /sessions/{id}/events?after=<seq>` returns `text/event-stream`. Its initial durable replay is bounded by `limit` (default `100`, maximum `500`), then the connection remains open for events created after subscription. To reconstruct a backlog larger than one page, first page through `/events/history`, advancing `after` to the last durable cursor; do not rely on the open stream to fill an older backlog beyond its initial page.

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
{ "session_id": "ses_...", "aborted": 2 }
```

`aborted` is the number of in-flight runs cancelled. Queued runs are not removed and remain scheduled. Aborts are idempotent — a 200 with `aborted: 0` is returned when no run is in flight. A 404 is returned only when the session id is unknown.

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

`POST /run` creates an in-memory session, streams the run, and does not persist the session or its messages. Unlike persistent session SSE, it forwards the raw run stream (including `stream_part`) and ends with `done` on success or `error` on a terminal failure; it cannot be replayed.

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
