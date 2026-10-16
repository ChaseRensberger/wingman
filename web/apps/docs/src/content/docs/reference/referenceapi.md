---
title: "API"
description: "HTTP operations, request rules, and response formats."
---

# API

The default server URL is `http://127.0.0.1:2424`. API paths start at the server root, outside `/console/`.
All routes except `/health` require [HTTP Basic Auth](/concepts/authentication).

Use `GET /openapi.json` for the complete endpoint and field reference.
It describes requests, responses, errors, and events in OpenAPI 3.1 format.
For connection examples, read [HTTP API Basics](/build-clients/http-api-basics).

## Request rules

Send JSON request bodies with `Content-Type: application/json`.
Most responses use JSON. Event streams use server-sent events (SSE), which send updates over an open HTTP connection.
Catalog logos return image data.

The standard request timeout is 60 seconds. Session event endpoints and `/run` bypass this timeout.
JSON errors contain `error.code`, `error.message`, and `error.request_id`.
The `X-Request-ID` response header contains the same request ID.

`X-Wingman-Client` selects the client identity for sessions and workspaces.
If omitted, Wingman uses `cli_wingclient`. Client identities do not provide separate credentials or access boundaries.

## Main operations

| Resource             | Operations                                                              |
| -------------------- | ----------------------------------------------------------------------- |
| Health               | `GET /health`, `GET /ready`                                             |
| Service              | `GET /`, `POST /service/restart`                                        |
| Agents               | `GET` or `POST /agents`, `GET`, `PUT`, or `DELETE /agents/{id}`         |
| Clients              | `GET` or `POST /clients`, `GET /clients/{id}`, `GET /client`            |
| Workspaces           | `GET` or `POST /workspaces`, `GET`, `PUT`, or `DELETE /workspaces/{id}` |
| Sessions             | `GET` or `POST /sessions`, `GET /sessions/{id}`                         |
| Messages             | `POST /sessions/{id}/message`                                           |
| Runs                 | `GET /sessions/{id}/runs`, `GET /sessions/{id}/runs/{runID}`            |
| Events               | `GET /sessions/{id}/events`, `GET /sessions/{id}/events/history`        |
| One-shot runs        | `POST /run`                                                             |
| Provider credentials | `GET` or `PUT /provider/auth`, `DELETE /provider/auth/{provider}`       |
| Providers and models | `GET /provider`, `GET /provider/{name}`, `GET /provider/{name}/models`  |
| Tool sources         | `GET /tools`, `GET /plugins`, `GET /mcp`                                |
| Diagnostics          | `GET /logs`, `GET /diagnostics`                                         |

`GET /ready` reports readiness, instance ID, and version. Readiness does not prove that provider credentials or credits are valid.
`POST /service/restart` requires `X-Wingman-Console: 1`. It returns `409` for a foreground server.
MCP OAuth authorization and logout endpoints return `501 Not Implemented`.

## Session responses

Create and list operations return session summaries. These contain metadata and a `version` that starts at `1`.
`GET /sessions/{id}` also returns `history` and, when available, `latest_model_call`.
An empty history is `[]`.

Create a session with either `working_directory` or `workspace_id`:

```json
{
  "title": "Research",
  "working_directory": "/home/me/project"
}
```

The directory must exist on the Wingman computer. A workspace supplies its path when you create or move the session.
Later workspace edits do not change existing sessions.

## Message request

Send this body to `POST /sessions/{id}/message`. Replace the agent ID with an existing ID:

```json
{
  "request_id": "submit-123",
  "agent_id": "agt_...",
  "message": "Summarize this project."
}
```

The session must exist. Wingman saves the request before it returns `202 Accepted`:

```json
{
  "run_id": "run_...",
  "status": "queued",
  "session_version": 2
}
```

`request_id` is optional, limited to 200 bytes, and unique within a session.
Retry the same effective input with the same ID to return the existing run.
Changing the prompt, effective agent or model, output schema, client, or current session location returns `409 Conflict`.
Without an ID, each request creates another run.

An identical retry also returns `202`. Its `status` and `session_version` reflect the current state.
Saved input includes the accepted agent, location, project instructions, and skills.

## Session changes

Read the session's current `version` before a change. A stale `expected_version` returns `409 Conflict`.

| Operation | Request                                                                                             |
| --------- | --------------------------------------------------------------------------------------------------- |
| Rename    | `POST /sessions/{id}/rename` with `title` and `expected_version`                                    |
| Move      | `POST /sessions/{id}/move` with `expected_version` and either `working_directory` or `workspace_id` |
| Delete    | `DELETE /sessions/{id}?expected_version=<version>`                                                  |

Rename and move return the updated summary. An unchanged title or location does not increase the version.
Deletion permanently removes the session and its records. Wingman cancels active work and closes its streams before deletion returns success.

## Run status and cancellation

Run statuses are `queued`, `running`, `completed`, `failed`, and `aborted`.
Runs execute in submission order within one session. See [Sessions](/concepts/sessions#run-status-and-recovery) for restart recovery.

`POST /sessions/{id}/abort` cancels the active run and leaves queued work scheduled.
Its response contains `session_id` and `aborted`, which is `1` if Wingman requested cancellation, otherwise `0`.
Retrying this operation can cancel a different run if the queue advances.

To target one run, use `POST /sessions/{id}/runs/{runID}/abort`.
A queued run returns `200`. A locally running run receives a cancellation signal and returns `202`.
A terminal run, or a running run that this server does not own, returns `409`.

## Tools and approvals

Read tool execution records at `GET /sessions/{id}/tool-uses`.
Use their Wingman `tlu_` IDs for identity. Provider `call_id` values can repeat.
See [Tools](/concepts/tools#durable-execution-lifecycle) for execution states.

Read pending approvals at `GET /sessions/{id}/permission-requests`.
Reply at `POST /sessions/{id}/permission-requests/{requestID}/reply` with `{"response":"once"}`, `"always"`, or `"reject"`.
Identical replies return the existing result. Conflicting replies return `409`.
See [Permissions](/configure/permissions) for approval behavior.

## One-shot runs

`POST /run` executes without saving a conversation. Include an inline `agent` or a saved `agent_id`, plus a `message`.
In `--ephemeral` server mode, only inline agents work.

```json
{
  "agent": {
    "name": "Assistant",
    "model_ref": "anthropic/claude-sonnet-5"
  },
  "message": "Explain what an HTTP API does."
}
```

The stream ends with `done` or `error` and cannot be replayed.
See [Streaming Events](/build-clients/streaming-events) for both stream formats.
