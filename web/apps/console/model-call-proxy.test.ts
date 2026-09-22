import { expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";

import { createWingmanClient } from "@wingman-actor/client";
import { formatModelCallDiagnostics } from "./src/lib/model-call-diagnostics";
import type { ModelCall } from "./src/lib/types";

test("split development proxies model-call diagnostics to the authenticated daemon", async () => {
  const call: ModelCall = {
    id: "mcl_proxy",
    session_id: "ses_proxy",
    step: 1,
    attempt: 1,
    status: "failed",
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: 0,
    context_tokens: 0,
    trace: {
      version: "1",
      model: { provider: "openai", id: "luna" },
      capabilities: {},
      runtime: { current_date: false },
      messages: { count: 1, by_role: { user: 1 }, part_kinds: { text: 1 } },
      system: { bytes: 0, sha256: "hash" },
      failure: {
        stage: "stream",
        category: "provider",
        http_status: 200,
        code: "unsupported_model",
        message: "Use the Responses API",
        body: '{"error":{"unknown":"preserved"}}',
        response_headers: { "x-request-id": ["req_proxy"] },
        causes: [{ type: "*net.OpError", message: "connection reset", code: "ECONNRESET" }],
        retryable: false,
        output_started: false,
      },
    },
  };
  const requests: string[] = [];
  const upstream = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      requests.push(new URL(request.url).pathname);
      if (request.headers.get("Authorization") !== `Basic ${btoa("diagnostic:test-password")}`) {
        return new Response("unauthorized", { status: 401 });
      }
      return Response.json([call]);
    },
  });
  const dir = mkdtempSync(join(tmpdir(), "wingman-diagnostic-proxy-"));
  mkdirSync(join(dir, "wingman"));
  writeFileSync(
    join(dir, "wingman", "registration.json"),
    JSON.stringify({ url: upstream.url.toString() }),
  );
  writeFileSync(
    join(dir, "wingman", "service.env"),
    "WINGMAN_USERNAME='diagnostic'\nWINGMAN_PASSWORD='test-password'\n",
  );
  const previous = { state: process.env.XDG_STATE_HOME, config: process.env.XDG_CONFIG_HOME };
  process.env.XDG_STATE_HOME = dir;
  process.env.XDG_CONFIG_HOME = dir;
  let vite: Awaited<ReturnType<typeof createServer>> | undefined;
  try {
    vite = await createServer({
      configFile: join(import.meta.dir, "vite.config.ts"),
      logLevel: "silent",
      server: { host: "127.0.0.1", port: 0, watch: null },
    });
    await vite.listen();
    const client = createWingmanClient({ baseUrl: new URL(vite.resolvedUrls!.local[0]!).origin });
    const calls = (await client.sessions.modelCalls.list("ses_proxy")) as ModelCall[];
    expect(requests).toEqual(["/sessions/ses_proxy/model-calls"]);
    expect(calls[0]?.trace?.failure?.code).toBe("unsupported_model");
    expect(calls[0]?.trace?.failure).toEqual(call.trace!.failure);
    expect(formatModelCallDiagnostics(calls[0]!)).toContain('"http_status": 200');
    expect(formatModelCallDiagnostics(calls[0]!)).toContain("Use the Responses API");
    expect(formatModelCallDiagnostics(calls[0]!)).toContain("ECONNRESET");
  } finally {
    await vite?.close();
    upstream.stop(true);
    if (previous.state === undefined) delete process.env.XDG_STATE_HOME;
    else process.env.XDG_STATE_HOME = previous.state;
    if (previous.config === undefined) delete process.env.XDG_CONFIG_HOME;
    else process.env.XDG_CONFIG_HOME = previous.config;
    rmSync(dir, { recursive: true, force: true });
  }
}, 30_000);
