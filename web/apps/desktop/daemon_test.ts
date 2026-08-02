import { DaemonDiscovery, proxyDaemonRequest } from "./daemon.ts";

function registration(instance = "ins_one", version = "1.0.0", url = "http://127.0.0.1:2323") {
  return JSON.stringify({ instance_id: instance, version, url, pid: 1, created_at: "2026-08-02T00:00:00Z" });
}

function newDiscovery(fetchResponses: Response[]) {
	return {
    discovery: new DaemonDiscovery({
      stateDir: () => "/state/wingman",
      readTextFile: async (path) => path.endsWith("registration.json") ? registration() : "credential\n",
      fetch: async () => fetchResponses.shift() ?? new Response(null, { status: 503 }),
      now: () => 0,
      timeoutSignal: () => AbortSignal.timeout(100),
    }),
	};
}

Deno.test("Desktop discovery retries a daemon that becomes ready", async () => {
	const { discovery } = newDiscovery([
    new Response(null, { status: 503 }),
    new Response(JSON.stringify({ ready: true, instance_id: "ins_one", version: "1.0.0" })),
  ]);
  await discovery.transport().then(
    () => { throw new Error("discovery succeeded before readiness"); },
    (error) => {
      if (!String(error).includes("not ready")) throw error;
    },
  );
  const transport = await discovery.transport();
  if (transport.origin !== "http://127.0.0.1:2323" || transport.credential !== "credential") {
    throw new Error(`transport = ${JSON.stringify(transport)}`);
  }
});

Deno.test("Desktop discovery rejects a stale registration identity", async () => {
	const { discovery } = newDiscovery([
    new Response(JSON.stringify({ ready: true, instance_id: "ins_other", version: "1.0.0" })),
  ]);
  await discovery.transport().then(
    () => { throw new Error("discovery accepted stale registration"); },
    (error) => {
      if (!String(error).includes("does not match registration")) throw error;
    },
  );
});

Deno.test("Desktop proxy rediscovers credentials after daemon authentication or readiness failures", async () => {
  for (const failureStatus of [401, 503]) {
    let generation = 1;
    const authorization: string[] = [];
    const fetchMock: typeof fetch = async (input) => {
      if (!(input instanceof Request)) {
        return new Response(JSON.stringify({ ready: true, instance_id: `ins_${generation}`, version: "1.0.0" }));
      }
      authorization.push(input.headers.get("Authorization") ?? "");
      if (authorization.length === 1) {
        generation = 2;
        return new Response(null, { status: failureStatus });
      }
      return new Response("ok");
    };
    const discovery = new DaemonDiscovery({
      stateDir: () => "/state/wingman",
      readTextFile: async (path) => {
        if (path.endsWith("registration.json")) return registration(`ins_${generation}`);
        return `credential_${generation}\n`;
      },
      fetch: fetchMock,
      now: () => 0,
      timeoutSignal: () => AbortSignal.timeout(100),
    });
    const request = new Request("http://127.0.0.1/health");
    const first = await proxyDaemonRequest(discovery, request, fetchMock);
    const second = await proxyDaemonRequest(discovery, request, fetchMock);
    if (first.status !== failureStatus || second.status !== 200 || authorization.join(",") !== "Bearer credential_1,Bearer credential_2") {
      throw new Error(`responses = ${first.status},${second.status}; authorization = ${authorization.join(",")}`);
    }
  }
});
