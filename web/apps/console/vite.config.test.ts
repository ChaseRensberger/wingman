import { expect, test } from "bun:test";
import { createServer, type ProxyOptions, type UserConfig } from "vite";

import config from "./vite.config";

test.each([
  { path: "/actions", data: [{ id: "compaction.compact", command: "compact" }] },
  { path: "/tools", data: { tools: [{ name: "read", directory_scoped: true, source: "native" }] } },
])("proxies $path outside the Console base path", async ({ path, data }) => {
  const consoleConfig = config as UserConfig;
  const proxy = consoleConfig.server?.proxy?.[path];
  expect(proxy).toBeDefined();
  expect(proxy).toEqual(consoleConfig.server?.proxy?.["/sessions"]);

  const requests: string[] = [];
  const daemon = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      requests.push(`${request.method} ${new URL(request.url).pathname}`);
      return Response.json(data);
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
        [path]: {
          ...(proxy as ProxyOptions),
          target: daemon.url.origin,
          headers: {},
        },
      },
    },
  });

  try {
    await vite.listen();
    const response = await fetch(new URL(path, vite.resolvedUrls!.local[0]));
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(data);
    expect(requests).toEqual([`GET ${path}`]);
  } finally {
    await vite.close();
    await daemon.stop(true);
  }
});
