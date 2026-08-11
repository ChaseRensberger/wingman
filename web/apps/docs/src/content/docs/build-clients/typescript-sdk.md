---
title: "TypeScript SDK"
description: "Use the typed Wingman REST and event-stream client."
---

# TypeScript SDK

Install the SDK version that matches your Wingman daemon release:

```bash
npm install @wingman-actor/client@0.1.41
```

The SDK is ESM-only. It supports Node.js 20 and later, Bun, and browser
bundlers with `fetch`, `ReadableStream`, and `TextDecoder` support.

## REST Requests

Create a client with the daemon URL and send the bearer token on each request:

```ts
import { apiData, createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: "http://localhost:2323",
  headers: {
    Authorization: `Bearer ${process.env.WINGMAN_TOKEN}`,
    "X-Wingman-Client": "my_client",
  },
});

const sessions = await apiData(client.GET("/sessions"));
```

`apiData` returns the response data for successful requests. It throws an
`APIError` for an HTTP error. The error has `status`, `code`, `message`,
`requestId`, and validation `details` fields.

```ts
import { APIError } from "@wingman-actor/client";

try {
  await apiData(client.POST("/sessions", { body: { title: "Research" } }));
} catch (error) {
  if (error instanceof APIError && error.code === "invalid_request") {
    console.error(error.details);
  }
  throw error;
}
```

Use a registered client bearer token outside trusted local administration. See
[HTTP API Basics](/build-clients/http-api-basics#authentication) for token and
client identity rules.

## One-Shot Streams

`streamRun` sends `POST /run` and yields typed stream envelopes. Pass an
`AbortSignal` to stop the request.

```ts
import { streamRun } from "@wingman-actor/client";

const controller = new AbortController();
for await (const result of streamRun(
  { model_ref: "openai/gpt-5.6-terra", message: "Summarize this project." },
  { baseUrl: "http://localhost:2323", signal: controller.signal },
)) {
  if (!result.known) continue;
  if (result.event.type === "stream_part") console.log(result.event.data);
  if (result.event.type === "error") throw new Error(result.event.data.message);
  if (result.event.type === "done") break;
}
```

Unknown event types are returned as `{ known: false, event }`. Ignore them so
that a newer daemon can add stream events without breaking the client.

## Persistent Session Streams

`streamSessionEvents` opens one `GET /sessions/{id}/events` connection. It
does not reconnect automatically because the application owns the durable
cursor and authoritative session state.

```ts
import { streamSessionEvents } from "@wingman-actor/client";

let lastSequence = loadLastSequence();
for await (const result of streamSessionEvents(sessionID, {
  baseUrl: "http://localhost:2323",
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

On a transport failure, reload the authoritative session and run. Reconnect
with the last saved durable sequence only while the run remains queued or
running. See [Streaming Events](/build-clients/streaming-events) for the full
event and recovery contract.

## Browser Use

The SDK can run in a browser, but the daemon does not enable cross-origin CORS.
Use it from the daemon's origin or through a same-origin backend proxy. Do not
place an owner credential in a remote browser application.

## Version Compatibility

The SDK is generated from the daemon OpenAPI contract. Until the API is stable,
use the exact SDK version that matches the daemon release tag. For example,
Wingman `v0.1.41` requires `@wingman-actor/client@0.1.41`.
