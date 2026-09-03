import { expect, test } from "bun:test";

import {
  APIError,
  StreamError,
  createWingmanClient,
  newActionAdmission,
  newMacroAdmission,
  newMessageAdmission,
  parseRunStreamEvent,
  parseSessionEvent,
  readSSE,
} from "../src/index";

test("readSSE handles split, multiline, and comment frames", async () => {
  const encoder = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(
        encoder.encode(': heartbeat\r\nid: 42\r\nevent: session.text.delta\r\ndata: {"delta":\r\n'),
      );
      controller.enqueue(encoder.encode('data: "hello"}\r\n\r\n'));
      controller.close();
    },
  });

  const events = await Array.fromAsync(readSSE(new Response(body)));
  expect(events).toEqual([{ id: "42", event: "session.text.delta", data: { delta: "hello" } }]);
});

test("parses known and unknown event envelopes", () => {
  const session = parseSessionEvent({
    id: "evt-1",
    type: "session.text.delta",
    schema_version: 1,
    data: { delta: "hi" },
  });
  expect(session?.known).toBe(true);
  const unknownSession = parseSessionEvent({
    id: "evt-2",
    type: "plugin.custom",
    schema_version: 1,
    data: { value: 1 },
  });
  expect(unknownSession).toEqual({
    known: false,
    event: {
      id: "evt-2",
      type: "plugin.custom",
      schema_version: 1,
      data: { value: 1 },
    },
  });

  const run = parseRunStreamEvent({
    type: "done",
    version: 1,
    data: { steps: 1 },
  });
  expect(run?.known).toBe(true);
  const unknownRun = parseRunStreamEvent({
    type: "extension",
    version: 1,
    data: { value: true },
  });
  expect(unknownRun).toEqual({
    known: false,
    event: { type: "extension", version: 1, data: { value: true } },
  });
});

test("configured clients send Basic authentication and client headers", async () => {
  let request: Request | undefined;
  const fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    request = new Request(input, init);
    return Response.json([]);
  };

  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    password: "secret",
    clientName: "console",
    headers: { "X-Trace-ID": "trace-1" },
    fetch,
  });
  await client.agents.list();
  expect(request?.url).toBe("https://wingman.test/agents");
  expect(request?.headers.get("Authorization")).toBe("Basic d2luZ21hbjpzZWNyZXQ=");
  expect(request?.headers.get("X-Wingman-Client")).toBe("console");
  expect(request?.headers.get("X-Trace-ID")).toBe("trace-1");
});

test("service restart sends the browser safety header", async () => {
  let request: Request | undefined;
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input, init) => {
      request = new Request(input, init);
      return Response.json({ status: "restarting" }, { status: 202 });
    },
  });

  await expect(client.service.restart()).resolves.toEqual({ status: "restarting" });
  expect(request?.method).toBe("POST");
  expect(request?.headers.get("X-Wingman-Console")).toBe("1");
});

test("resource methods return data and throw APIError", async () => {
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input) =>
      (input instanceof Request ? input.url : String(input)).endsWith("/agents")
        ? Response.json([{ id: "agent-1", name: "Assistant" }])
        : Response.json(
            {
              error: {
                code: "not_found",
                message: "missing",
                request_id: "req-1",
              },
            },
            { status: 404 },
          ),
  });
  await expect(client.agents.list()).resolves.toEqual([{ id: "agent-1", name: "Assistant" }]);
  await expect(client.agents.get("missing")).rejects.toMatchObject({
    name: "APIError",
    status: 404,
    code: "not_found",
    requestId: "req-1",
  } satisfies Partial<APIError>);
});

test("APIError keeps retry metadata", async () => {
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async () =>
      new Response(JSON.stringify({ error: { code: "busy", message: "retry" } }), {
        status: 429,
        headers: { "Retry-After": "3", "X-Request-ID": "req-1" },
      }),
  });

  await expect(client.agents.list()).rejects.toMatchObject({
    name: "APIError",
    retryAfterMs: 3000,
    requestId: "req-1",
  } satisfies Partial<APIError>);
});

test("client ensure creates or verifies a client identity", async () => {
  let requests = 0;
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input) => {
      requests++;
      const url = input instanceof Request ? input.url : String(input);
      if (requests === 1) {
        return Response.json(
          { error: { code: "conflict", message: "client exists" } },
          { status: 409 },
        );
      }
      expect(url).toBe("https://wingman.test/clients/cli_wingcode");
      return Response.json({
        id: "cli_wingcode",
        name: "Wingcode",
        created_at: "now",
      });
    },
  });

  await expect(client.clients.ensure(" cli_wingcode ", " Wingcode ")).resolves.toMatchObject({
    id: "cli_wingcode",
    name: "Wingcode",
  });
});

test("client ensure rejects a mismatched client identity", async () => {
  let requests = 0;
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async () => {
      requests++;
      if (requests === 1) {
        return Response.json(
          { error: { code: "conflict", message: "client exists" } },
          { status: 409 },
        );
      }
      return Response.json({
        id: "cli_wingcode",
        name: "Other",
        created_at: "now",
      });
    },
  });

  await expect(client.clients.ensure("cli_wingcode", "Wingcode")).rejects.toMatchObject({
    name: "APIError",
    status: 409,
    code: "conflict",
  } satisfies Partial<APIError>);
});

test("client stream methods inherit configured fetch and headers", async () => {
  let request: Request | undefined;
  const fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    request = new Request(input, init);
    return new Response(
      'data: {"id":"evt-1","type":"session.events.synchronized","schema_version":1,"data":{}}\n\n',
      { headers: { "Content-Type": "text/event-stream; charset=utf-8" } },
    );
  };

  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    clientName: "console",
    headers: { "X-Trace-ID": "trace-1" },
    fetch,
  });
  const events = await Array.fromAsync(
    client.sessions.streamEvents("ses /1", {
      after: 42,
      lastEventID: 41,
    }),
  );
  expect(request?.url).toBe("https://wingman.test/sessions/ses%20%2F1/events?after=42");
  expect(request?.headers.get("Last-Event-ID")).toBe("41");
  expect(request?.headers.get("X-Wingman-Client")).toBe("console");
  expect(request?.headers.get("X-Trace-ID")).toBe("trace-1");
  expect(events[0]?.known).toBe(true);
});

test("readiness requests receive their abort signal", async () => {
  let request: Request | undefined;
  const controller = new AbortController();
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input, init) => {
      request = new Request(input, init);
      return Response.json({ ready: true });
    },
  });

  await client.health.ready({ signal: controller.signal });
  controller.abort();
  expect(request?.signal.aborted).toBe(true);
});

test("stream requests reject non-SSE responses", async () => {
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async () => Response.json({ message: "proxy response" }),
  });

  await expect(
    client.run.stream({ model_ref: "openai/gpt-5.6-terra", message: "hello" }).next(),
  ).rejects.toMatchObject({
    name: "StreamError",
    code: "invalid_stream",
  } satisfies Partial<StreamError>);
});

test("run streams require a terminal event", async () => {
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async () =>
      new Response('data: {"type":"stream_part","version":1,"data":{}}\n\n', {
        headers: { "Content-Type": "text/event-stream" },
      }),
  });

  await expect(
    Array.fromAsync(
      client.run.stream({
        model_ref: "openai/gpt-5.6-terra",
        message: "hello",
      }),
    ),
  ).rejects.toMatchObject({
    name: "StreamError",
    code: "stream_interrupted",
  } satisfies Partial<StreamError>);
});

test("readSSE rejects oversized events", async () => {
  await expect(
    Array.fromAsync(readSSE(new Response("data: 123456789\n\n"), 8)),
  ).rejects.toMatchObject({
    name: "StreamError",
    code: "stream_too_large",
  } satisfies Partial<StreamError>);
});

test("newMessageAdmission creates and preserves request IDs", () => {
  const request = { agent_id: "agt_1", message: "hello" };
  const admitted = newMessageAdmission(request);
  expect(admitted.request_id).toBeString();
  expect(newMessageAdmission({ ...request, request_id: "req_1" })).toEqual({
    ...request,
    request_id: "req_1",
  });
});

test("newMacroAdmission creates and preserves request IDs", () => {
  const request = { agent_id: "agt_1", macro_id: "review" };
  const admitted = newMacroAdmission(request);
  expect(admitted.request_id).toBeString();
  expect(newMacroAdmission({ ...request, request_id: "req_1" })).toEqual({
    ...request,
    request_id: "req_1",
  });
});

test("macro admission posts the macro ID and arguments", async () => {
  let request: Request | undefined;
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input, init) => {
      request = new Request(input, init);
      return Response.json({ run_id: "run_1", status: "queued", session_version: 2 });
    },
  });

  await expect(
    client.sessions.macros.admit("ses_1", {
      request_id: "req_1",
      macro_id: "review/security",
      arguments: "auth middleware",
      agent_id: "agt_1",
    }),
  ).resolves.toMatchObject({ run_id: "run_1" });
  expect(request?.url).toBe("https://wingman.test/sessions/ses_1/macros");
  expect(request?.method).toBe("POST");
  expect(await request?.json()).toEqual({
    request_id: "req_1",
    macro_id: "review/security",
    arguments: "auth middleware",
    agent_id: "agt_1",
  });
});

test("action discovery and admission use the generic action contract", async () => {
  const requests: Request[] = [];
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async (input, init) => {
      const request = new Request(input, init);
      requests.push(request);
      if (request.method === "GET") {
        return Response.json({
          actions: [{ id: "compaction.compact", command: "compact" }],
        });
      }
      return Response.json({ run_id: "run_1", status: "queued", session_version: 2 });
    },
  });

  await expect(client.actions.list()).resolves.toEqual([
    { id: "compaction.compact", command: "compact" },
  ]);
  await expect(
    client.sessions.actions.admit("ses_1", "compaction.compact", {
      request_id: "req_1",
      agent_id: "agt_1",
      input: { reason: "manual" },
    }),
  ).resolves.toMatchObject({ run_id: "run_1" });
  expect(requests[1]?.url).toBe("https://wingman.test/sessions/ses_1/actions/compaction.compact");
  expect(await requests[1]?.json()).toEqual({
    request_id: "req_1",
    agent_id: "agt_1",
    input: { reason: "manual" },
  });
});

test("newActionAdmission creates and preserves request IDs", () => {
  const generated = newActionAdmission({ agent_id: "agt_1" });
  expect(generated.request_id).toBeTruthy();
  expect(newActionAdmission(generated)).toBe(generated);
});

test("admit requires a request ID", () => {
  const client = createWingmanClient({ baseUrl: "https://wingman.test" });
  expect(() => client.sessions.admit("ses_1", { agent_id: "agt_1", message: "hello" })).toThrow(
    "message admission requires a request_id",
  );
  expect(() =>
    client.sessions.actions.admit("ses_1", "compaction.compact", { agent_id: "agt_1" }),
  ).toThrow("action admission requires a request_id");
});

test("clients reject non-origin URLs", () => {
  expect(() => createWingmanClient({ baseUrl: "https://wingman.test/api" })).toThrow(
    "base URL must be an HTTP or HTTPS origin",
  );
});

test("stream requests expose API errors", async () => {
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    fetch: async () =>
      new Response(
        JSON.stringify({
          error: { code: "unauthorized", message: "no token" },
        }),
        { status: 401 },
      ),
  });
  await expect(
    client.run.stream({ model_ref: "openai/gpt-5.6-terra", message: "hello" }).next(),
  ).rejects.toMatchObject({
    name: "APIError",
    status: 401,
    code: "unauthorized",
  } satisfies Partial<APIError>);
});
