import { expect, test } from "bun:test";

import {
  APIError,
  apiData,
  parseRunStreamEvent,
  parseSessionEvent,
  readSSE,
  streamRun,
  streamSessionEvents,
} from "../src/index";

test("readSSE handles split, multiline, and comment frames", async () => {
  const encoder = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(": heartbeat\r\nid: 42\r\nevent: session.text.delta\r\ndata: {\"delta\":\r\n"));
      controller.enqueue(encoder.encode("data: \"hello\"}\r\n\r\n"));
      controller.close();
    },
  });

  const events = await Array.fromAsync(readSSE(new Response(body)));
  expect(events).toEqual([{ id: "42", event: "session.text.delta", data: { delta: "hello" } }]);
});

test("parses known and unknown event envelopes", () => {
  const session = parseSessionEvent({ id: "evt-1", type: "session.text.delta", schema_version: 1, data: { delta: "hi" } });
  expect(session?.known).toBe(true);
  const unknownSession = parseSessionEvent({ id: "evt-2", type: "plugin.custom", schema_version: 1, data: { value: 1 } });
  expect(unknownSession).toEqual({ known: false, event: { id: "evt-2", type: "plugin.custom", schema_version: 1, data: { value: 1 } } });

  const run = parseRunStreamEvent({ type: "done", version: 1, data: { steps: 1 } });
  expect(run?.known).toBe(true);
  const unknownRun = parseRunStreamEvent({ type: "extension", version: 1, data: { value: true } });
  expect(unknownRun).toEqual({ known: false, event: { type: "extension", version: 1, data: { value: true } } });
});

test("streamRun sends JSON and yields typed events", async () => {
  let request: Request | undefined;
  const fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    request = new Request(input, init);
    return new Response('event: done\ndata: {"type":"done","version":1,"data":{"steps":1}}\n\n');
  };

  const events = await Array.fromAsync(streamRun({ model_ref: "openai/gpt-5.6-terra", message: "hello" }, { baseUrl: "https://wingman.test", fetch }));
  expect(request?.url).toBe("https://wingman.test/run");
  expect(request?.headers.get("Accept")).toBe("text/event-stream");
  expect(request?.headers.get("Content-Type")).toBe("application/json");
  expect(await request?.json()).toEqual({ model_ref: "openai/gpt-5.6-terra", message: "hello" });
  expect(events[0]?.known).toBe(true);
});

test("streamSessionEvents sends the cursor and client headers", async () => {
  let request: Request | undefined;
  const fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    request = new Request(input, init);
    return new Response('data: {"id":"evt-1","type":"session.events.synchronized","schema_version":1,"data":{}}\n\n');
  };

  const events = await Array.fromAsync(streamSessionEvents("ses /1", {
    after: 42,
    lastEventID: 41,
    baseUrl: "https://wingman.test/api/",
    headers: { "X-Wingman-Client": "console" },
    fetch,
  }));
  expect(request?.url).toBe("https://wingman.test/sessions/ses%20%2F1/events?after=42");
  expect(request?.headers.get("Last-Event-ID")).toBe("41");
  expect(request?.headers.get("X-Wingman-Client")).toBe("console");
  expect(events[0]?.known).toBe(true);
});

test("apiData and stream requests expose API errors", async () => {
  await expect(apiData(Promise.resolve({
    error: { error: { code: "invalid_request", message: "message is required", request_id: "req-1" } },
    response: new Response("", { status: 400 }),
  }))).rejects.toMatchObject({
    name: "APIError",
    status: 400,
    code: "invalid_request",
    requestId: "req-1",
  } satisfies Partial<APIError>);

  const events = streamRun({ model_ref: "openai/gpt-5.6-terra", message: "hello" }, {
    fetch: async () => new Response(JSON.stringify({ error: { code: "unauthorized", message: "no token" } }), { status: 401 }),
  });
  await expect(events.next()).rejects.toMatchObject({ name: "APIError", status: 401, code: "unauthorized" } satisfies Partial<APIError>);
});
