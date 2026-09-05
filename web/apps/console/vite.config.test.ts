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
