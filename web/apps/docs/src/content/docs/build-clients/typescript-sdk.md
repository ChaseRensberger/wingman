---
title: "TypeScript SDK"
description: "Use the typed Wingman REST and event-stream client."
---

# TypeScript SDK

Install the SDK version that matches the Wingman daemon release:

```bash
npm install @wingman-actor/client@0.1.41
```

The SDK is ESM-only. It supports Node.js 20 and later, Bun, and browser
bundlers that support `fetch`, `ReadableStream`, and `TextDecoder`.

## REST Requests

Create a client with the daemon URL, password, and client identity. The client
uses this configuration for REST requests and streams.

```ts
import { createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: "http://localhost:2323",
  password: process.env.WINGMAN_DAEMON_PASSWORD,
  clientName: "cli_wingcode",
});

await client.clients.ensure("cli_wingcode", "Wingcode");
const sessions = await client.sessions.list();
```

`clients.ensure` creates the client identity at the first start. On later
starts, it gets and compares the existing identity. If the name differs, it
throws an `APIError` with the `conflict` code.

Resource methods return response data for successful requests. They throw an
`APIError` for an HTTP error. The error contains `status`, `code`, `message`,
`requestId`, validation `details`, response `headers`, and `retryAfterMs`
fields.

```ts
import { APIError } from "@wingman-actor/client";

try {
  await client.sessions.create({ title: "Research" });
} catch (error) {
  if (error instanceof APIError && error.code === "invalid_request") {
    console.error(error.details);
  }
  throw error;
}
```

Use TLS or an SSH tunnel before sending the daemon password to a remote server.
See [HTTP API Basics](/build-clients/http-api-basics#authentication) for
authentication and client identity rules.

The base URL must be an HTTP or HTTPS origin. Do not include a path, query,
fragment, or credentials.

## Admit Messages Safely

Persistent message admission is idempotent when each retry uses the same
`request_id`. `newMessageAdmission` adds an ID when the request has none.

Save the returned request before the first network request:

```ts
import { newMessageAdmission } from "@wingman-actor/client";

const request = newMessageAdmission({
  agent_id: "agt_assistant",
  message: "Summarize this project.",
});
await savePendingRequest(sessionID, request);

const admission = await client.sessions.admit(sessionID, request);
await deletePendingRequest(sessionID, request);
```

If the request result is unknown, retry `client.sessions.admit` with the saved
request. Do not create a new request ID for this retry.

## One-Shot Streams

`client.run.stream` sends `POST /run` and returns typed stream envelopes. Pass
an `AbortSignal` to stop the request. It throws `StreamError` when the server
does not return SSE or ends before a terminal event.

```ts
const controller = new AbortController();
for await (const result of client.run.stream(
  { model_ref: "openai/gpt-5.6-terra", message: "Summarize this project." },
  { signal: controller.signal },
)) {
  if (!result.known) continue;
  if (result.event.type === "stream_part") console.log(result.event.data);
  if (result.event.type === "error") throw new Error(result.event.data.message);
  if (result.event.type === "done") break;
}
```

Unknown event types return as `{ known: false, event }`. Ignore them. A newer
daemon can then add stream events without breaking the client.

## Persistent Session Streams

`client.sessions.streamEvents` opens one `GET /sessions/{id}/events`
connection. It does not reconnect automatically. The application owns the
durable cursor and authoritative session state.

```ts
let lastSequence = loadLastSequence();
for await (const result of client.sessions.streamEvents(sessionID, {
  after: lastSequence,
  lastEventID: lastSequence || undefined,
})) {
  const event = result.event;
  if (event.cursor) {
    lastSequence = Math.max(lastSequence, event.cursor.seq);
    saveLastSequence(lastSequence);
  }
  if (event.type === "session.events.synchronized") continue;
  if (event.type === "session.events.resync_required") {
    await reloadSessionAndRun();
    break;
  }
  if (result.known) applyEvent(event);
}
```

If transport fails, reload the authoritative session and run. Reconnect with
the last saved durable sequence only while the run remains queued or running.
Read [Streaming Events](/build-clients/streaming-events) for the complete event
and recovery contract.

## Browser Use

The SDK can run in a browser. The daemon does not enable cross-origin CORS. Use
it from the daemon's origin or through a same-origin backend proxy. Do not
place an owner credential in a remote browser application.

## Version Compatibility

The SDK is generated from the daemon OpenAPI contract. Until the API is stable,
use the exact SDK version that matches the daemon release tag. For example,
Wingman `v0.1.41` requires `@wingman-actor/client@0.1.41`.
