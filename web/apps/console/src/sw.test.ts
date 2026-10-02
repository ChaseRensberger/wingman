import { beforeAll, expect, test } from "bun:test";
import { runInNewContext } from "node:vm";

let source: string;
beforeAll(async () => {
  const build = await Bun.build({
    entrypoints: [new URL("./sw.ts", import.meta.url).pathname],
    format: "iife",
    define: { "process.env.NODE_ENV": '"production"' },
  });
  if (!build.success) throw new Error(build.logs.join("\n"));
  source = await build.outputs[0].text();
});

function worker(installed = true, missingAsset = false) {
  const origin = "https://wingman.example";
  const listeners = new Map<string, Array<(event: any) => void>>();
  const cached = new Map<string, Response>();
  if (installed) {
    cached.set(`${origin}/console/index.html?__WB_REVISION__=test`, new Response("cached shell"));
    cached.set(
      `${origin}/console/assets/app.js?__WB_REVISION__=test`,
      new Response("cached asset"),
    );
  }
  const key = (request: Request | string) => (typeof request === "string" ? request : request.url);
  class WorkerEvent {
    pending: Promise<unknown>[] = [];
    constructor(public type: string) {}
    waitUntil(value: Promise<unknown>) {
      void value.catch(() => {});
      this.pending.push(value);
    }
  }
  class WorkerFetchEvent extends WorkerEvent {}
  let online = true;
  let status = 200;
  const context: Record<string, any> = {
    URL,
    Request,
    Response,
    Headers,
    console,
    setTimeout,
    ExtendableEvent: WorkerEvent,
    FetchEvent: WorkerFetchEvent,
    location: new URL(`${origin}/console/sw.js`),
    __WB_MANIFEST: [
      { url: "/console/index.html", revision: "test" },
      { url: "/console/assets/app.js", revision: "test" },
    ],
    registration: {
      scope: `${origin}/console/`,
    },
    caches: {
      match: async (request: Request | string) => cached.get(key(request))?.clone(),
      open: async () => ({
        match: async (request: Request | string) => cached.get(key(request))?.clone(),
        put: async (request: Request | string, response: Response) => {
          cached.set(key(request), response.clone());
        },
      }),
    },
    fetch: async (request: Request) => {
      if (!online) throw new TypeError("offline");
      const script = new URL(request.url).pathname.endsWith(".js");
      return new Response("network", {
        status,
        headers: {
          "Content-Type": script && !missingAsset ? "application/javascript" : "text/html",
        },
      });
    },
    addEventListener: (type: string, listener: (event: any) => void) => {
      listeners.set(type, [...(listeners.get(type) ?? []), listener]);
    },
  };
  context.self = context;
  runInNewContext(source, context);
  return {
    cached,
    offline() {
      online = false;
    },
    unauthorized() {
      status = 401;
    },
    async request(path: string, mode = "cors", method = "GET") {
      const request = new Request(`${origin}${path}`, { method });
      Object.defineProperty(request, "mode", { value: mode });
      let result: Promise<Response> | undefined;
      for (const listener of listeners.get("fetch") ?? [])
        listener({
          request,
          respondWith: (response: Promise<Response>) => {
            result = response;
          },
        });
      return result;
    },
    async install() {
      const event = new WorkerEvent("install");
      for (const listener of listeners.get("install") ?? []) listener(event);
      await Promise.all(event.pending);
    },
  };
}

test("worker caches only interface assets and preserves authentication responses", async () => {
  const sw = worker();
  expect(await (await sw.request("/console/sessions/ses_1", "navigate"))?.text()).toBe("network");
  sw.unauthorized();
  expect((await sw.request("/console/", "navigate"))?.status).toBe(401);
  sw.offline();
  expect(await (await sw.request("/console/sessions/ses_1", "navigate"))?.text()).toBe(
    "cached shell",
  );
  expect(await (await sw.request("/console/assets/app.js"))?.text()).toBe("cached asset");
  for (const path of ["/ready", "/sessions/ses_1/events", "/sessions/ses_1/message"]) {
    expect(await sw.request(path)).toBeUndefined();
    expect(await sw.request(path, "cors", "POST")).toBeUndefined();
  }
  expect(await sw.request("/console/", "cors", "POST")).toBeUndefined();
});

test("worker installs a complete build but rejects HTML returned for a missing asset", async () => {
  const valid = worker(false);
  await valid.install();
  expect(valid.cached.size).toBe(2);
  const incomplete = worker(false, true);
  await expect(incomplete.install()).rejects.toThrow("bad-precaching-response");
  expect([...incomplete.cached.keys()].some((url) => url.includes("app.js"))).toBe(false);
});
