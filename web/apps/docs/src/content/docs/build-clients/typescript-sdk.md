---
title: "TypeScript SDK"
description: "Call Wingman and read event streams from TypeScript."
---

# TypeScript SDK

Use the TypeScript SDK to call Wingman's HTTP API and read event streams.

See the [TypeScript Client API](/reference/typescript-client-api/) for the
complete public method index.

## Install

Install the SDK version that matches the Wingman daemon:

```bash
npm install @wingman-actor/client@0.1.64
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

Call `clients.ensure` at startup to create or reuse a client identity:

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

Retrying the same input with the same `request_id` does not create another run.
`newMessageAdmission` adds an ID if the request has none.

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

`client.run.stream` sends `POST /run` and reads server-sent events (SSE).
Pass an `AbortSignal` to stop the request.
It throws `StreamError` if the response is not SSE or ends before `done` or `error`.

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

Unknown event types return as `{ known: false, event }`. Ignore them.

## Persistent Session Streams

`client.sessions.streamEvents` opens one `GET /sessions/{id}/events` connection.
It does not reconnect automatically. Your application must save the last event sequence and reload session state.

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

After a disconnect, reload the session and run. Reconnect from the last saved sequence if the run is queued or running.
See [Streaming Events](/build-clients/streaming-events) for recovery steps.

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

Use the SDK version that matches the server release.
For Wingman `v0.1.64`, use `@wingman-actor/client@0.1.64`.
