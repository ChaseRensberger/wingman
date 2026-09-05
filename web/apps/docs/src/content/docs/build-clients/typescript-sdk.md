---
title: "TypeScript SDK"
description: "Use the typed Wingman REST and event-stream client."
---

# TypeScript SDK

The TypeScript SDK provides typed REST methods and typed Wingman event streams.

See the [TypeScript Client API](/reference/typescript-client-api/) for the
complete public method index.

## Install

Install the SDK version that matches the Wingman daemon:

```bash
npm install @wingman-actor/client@0.1.58
```

The SDK is ESM-only. It supports Node.js 20 and later, Bun, and browser
bundlers with `fetch`, `ReadableStream`, and `TextDecoder`.

## Connect

### Local Managed Daemon

For a local Node.js or Bun application, export the managed daemon credentials
and registered URL. These commands require `jq`:

```bash
set -a
source "${XDG_CONFIG_HOME:-$HOME/.config}/wingman/service.env"
WINGMAN_URL=$(jq -r .url "${XDG_STATE_HOME:-$HOME/.local/state}/wingman/registration.json")
set +a
```

### Explicit Server

For a foreground or remote server, set the URL and credentials from that
server. The base URL must be an HTTP or HTTPS origin. Do not include a path,
query, fragment, or credentials.

Before you send Basic Auth credentials to a remote server, use TLS or an SSH
tunnel. Read [Authentication](/concepts/authentication) for credential and
security details.

```ts
import { createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: process.env.WINGMAN_URL!,
  username: process.env.WINGMAN_USERNAME,
  password: process.env.WINGMAN_PASSWORD,
  clientName: "cli_wingcode",
});
```

`username` is optional. The server defaults it to `wingman`.

## Client Identity

At application startup, call `clients.ensure`. It creates the client identity
on the first start. On later starts, it reads and compares the existing
identity.

```ts
await client.clients.ensure("cli_wingcode", "Wingcode");
```

If the name differs, `clients.ensure` throws an `APIError` with the `conflict`
code.

`clientName` sets the default `X-Wingman-Client` header. A request header
overrides this value.

## REST Requests

Resource methods return response data for successful requests. They throw an
`APIError` for HTTP errors:

```ts
const sessions = await client.sessions.list();
```

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
daemon can add stream events without breaking the client.

## Persistent Session Streams

`client.sessions.streamEvents` opens one `GET /sessions/{id}/events` connection.
It does not reconnect automatically. The application owns the durable cursor
and authoritative session state.

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

If transport fails, reload the authoritative session and run. Reconnect only
while the run is queued or running. Use the last saved durable sequence. Read
[Streaming Events](/build-clients/streaming-events) for the event recovery contract.

## Handle Errors

Non-success responses throw `APIError`. It includes `status`, `code`,
`message`, `requestId`, validation `details`, response `headers`, and
`retryAfterMs`.

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

## Browser Use

The SDK can run in a browser. The daemon does not enable cross-origin CORS. Use
it from the daemon origin or a same-origin backend proxy. Do not place an owner
credential in a remote browser application.

## Version Compatibility

The SDK is generated from the daemon OpenAPI contract. Until the API is stable,
use the exact SDK version that matches the daemon release tag. For example,
Wingman `v0.1.58` requires `@wingman-actor/client@0.1.58`.
