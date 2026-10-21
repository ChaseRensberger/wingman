import { expect, test } from "bun:test";
import { createServer, type ProxyOptions, type UserConfig } from "vite";
import { createWingmanClient, type SessionEvent } from "@wingman-actor/client";
import config from "./vite.config";

test("split development routes queue admission, recovery, notices, and stop to the authenticated API", async () => {
  const notice: SessionEvent = {
    id: "notice",
    schema_version: 1,
    type: "session.agent.switched",
    cursor: { session_id: "s", seq: 3 },
    data: {
      run_id: "next",
      agent: "build",
      name: "Build",
      previous: "plan",
      previous_name: "Plan",
    },
  };
  const requests: string[] = [];
  const daemon = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const path = new URL(request.url).pathname;
      if (request.headers.get("Authorization") !== "Basic cXVldWU6c2VjcmV0")
        return new Response("Unauthorized", { status: 401 });
      requests.push(`${request.method} ${path}`);
      if (path.endsWith("/actions/compaction.compact")) {
        expect(await request.json()).toEqual({
          request_id: "retry",
          agent_id: "build",
          model_ref: "test/model",
          input: { arguments: "keep recent changes", limit: 10 },
        });
        return Response.json(
          { run_id: "retried", status: "queued", session_version: 3 },
          { status: 202 },
        );
      }
      if (path.endsWith("/message")) {
        expect(await request.json()).toEqual({
          request_id: "request",
          agent_id: "build",
          model_ref: "test/model",
          message: "next",
        });
        return Response.json(
          { run_id: "next", status: "queued", session_version: 2 },
          { status: 202 },
        );
      }
      if (path.endsWith("/events"))
        return new Response(`event: ${notice.type}\ndata: ${JSON.stringify(notice)}\n\n`, {
          headers: { "Content-Type": "text/event-stream" },
        });
      if (path.endsWith("/runs"))
        return Response.json([
          { id: "active", status: "running" },
          { id: "next", status: "queued", agent: { id: "build", model_ref: "test/model" } },
        ]);
      if (path.endsWith("/abort")) return new Response(null, { status: 204 });
      return Response.json({ id: "s", history: [], notices: [notice] });
    },
  });
  const consoleConfig = config as UserConfig;
  const vite = await createServer({
    configFile: false,
    base: consoleConfig.base,
    optimizeDeps: { noDiscovery: true, include: [] },
    server: {
      host: "127.0.0.1",
      port: 0,
      watch: null,
      ws: false,
      proxy: {
        "/sessions": {
          ...(consoleConfig.server?.proxy?.["/sessions"] as ProxyOptions),
          target: daemon.url.origin,
          headers: { Authorization: "Basic cXVldWU6c2VjcmV0" },
        },
      },
    },
  });
  try {
    await vite.listen();
    const client = createWingmanClient({ baseUrl: new URL(vite.resolvedUrls!.local[0]!).origin });
    expect(
      await client.sessions.admit("s", {
        request_id: "request",
        agent_id: "build",
        model_ref: "test/model",
        message: "next",
      }),
    ).toMatchObject({ run_id: "next", status: "queued" });
    expect((await client.sessions.runs.list("s"))[1]).toMatchObject({
      id: "next",
      agent: { id: "build", model_ref: "test/model" },
    });
    expect((await client.sessions.get("s")).notices).toEqual([notice]);
    for await (const parsed of client.sessions.streamEvents("s")) {
      expect(parsed.known).toBe(true);
      expect(parsed.event).toEqual(notice);
      break;
    }
    await client.sessions.runs.abort("s", "active");
    expect(
      await client.sessions.actions.admit("s", "compaction.compact", {
        request_id: "retry",
        agent_id: "build",
        model_ref: "test/model",
        input: { arguments: "keep recent changes", limit: 10 },
      }),
    ).toMatchObject({ run_id: "retried", status: "queued" });
    expect(requests).toEqual([
      "POST /sessions/s/message",
      "GET /sessions/s/runs",
      "GET /sessions/s",
      "GET /sessions/s/events",
      "POST /sessions/s/runs/active/abort",
      "POST /sessions/s/actions/compaction.compact",
    ]);
  } finally {
    await vite.close();
    await daemon.stop(true);
  }
});
