import { expect, test } from "bun:test";
import { createServer, type ProxyOptions, type UserConfig } from "vite";

import config from "./vite.config";

test("proxies the session action catalog outside the Console base path", async () => {
  const consoleConfig = config as UserConfig;
  const proxy = consoleConfig.server?.proxy?.["/actions"];
  expect(proxy).toBeDefined();
  expect(proxy).toEqual(consoleConfig.server?.proxy?.["/sessions"]);

  const actions = [{ id: "compaction.compact", command: "compact" }];
  const requests: string[] = [];
  const daemon = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      requests.push(`${request.method} ${new URL(request.url).pathname}`);
      return Response.json(actions);
    },
  });

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
        "/actions": {
          ...(proxy as ProxyOptions),
          target: daemon.url.origin,
          headers: {},
        },
      },
    },
  });

  try {
    await vite.listen();
    const response = await fetch(new URL("/actions", vite.resolvedUrls!.local[0]));
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(actions);
    expect(requests).toEqual(["GET /actions"]);
  } finally {
    await vite.close();
    await daemon.stop(true);
  }
});

test("forwards trigger CRUD and preview requests with daemon authentication at the server root", async () => {
  const consoleConfig = config as UserConfig;
  const proxy = consoleConfig.server?.proxy?.["/triggers"] as ProxyOptions;
  expect(proxy).toEqual(consoleConfig.server?.proxy?.["/sessions"]);
  const requests: Array<{ path: string; method: string; auth: string | null; body: string }> = [];
  const authorization = `Basic ${Buffer.from("wingman:test-trigger-password").toString("base64")}`;
  const daemon = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      requests.push({
        path: new URL(request.url).pathname,
        method: request.method,
        auth: request.headers.get("Authorization"),
        body: await request.text(),
      });
      return Response.json({ status: "ok" });
    },
  });
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
        "/triggers": {
          ...proxy,
          target: daemon.url.origin,
          headers: { Authorization: authorization },
        },
      },
    },
  });
  try {
    await vite.listen();
    for (const [method, path] of [
      ["GET", "/triggers"],
      ["POST", "/triggers"],
      ["POST", "/triggers/preview"],
      ["PUT", "/triggers/trg_test/state"],
      ["POST", "/triggers/trg_test/fire"],
      ["GET", "/triggers/trg_test/occurrences"],
      ["DELETE", "/triggers/trg_test?expected_version=1"],
    ]) {
      const body =
        method === "POST" || method === "PUT" ? JSON.stringify({ request_id: "test" }) : undefined;
      const response = await fetch(new URL(path, vite.resolvedUrls!.local[0]), {
        method,
        body,
        headers: body ? { "Content-Type": "application/json" } : undefined,
      });
      expect(response.status).toBe(200);
      expect(requests.at(-1)).toEqual({
        path: path.split("?")[0],
        method,
        auth: authorization,
        body: body ?? "",
      });
    }
  } finally {
    await vite.close();
    await daemon.stop(true);
  }
});
