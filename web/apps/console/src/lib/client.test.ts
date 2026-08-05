import { afterEach, describe, expect, test } from "bun:test";

import { APIError, apiData, isDaemonConnectionFailure, rotateClientToken } from "./client";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("generated API adapter", () => {
  test("returns generated response data", async () => {
    const data = await apiData(Promise.resolve({
      data: { status: "ok" },
      response: new Response(null, { status: 200 }),
    }));
    expect(data).toEqual({ status: "ok" });
  });

  test("preserves canonical API error fields", async () => {
    const request = apiData(Promise.resolve({
      error: {
        error: {
          code: "invalid_request",
          message: "invalid request",
          request_id: "req-1",
          details: [{ field: "name", reason: "required" }],
        },
      },
      response: new Response(null, { status: 400 }),
    }));
    const error = await request.catch((value) => value);
    expect(error).toBeInstanceOf(APIError);
    expect(error).toMatchObject({ status: 400, code: "invalid_request", requestId: "req-1" });
  });

  test("preserves plain-text proxy errors", async () => {
    const request = apiData(Promise.resolve({
      error: "Wingman daemon is unavailable",
      response: new Response(null, { status: 503 }),
    }));
    const error = await request.catch((value) => value);
    expect(error).toMatchObject({ status: 503, message: "Wingman daemon is unavailable" });
  });

  test("treats structured and plain server failures as daemon outages", () => {
    expect(isDaemonConnectionFailure(503)).toBe(true);
    expect(isDaemonConnectionFailure(500)).toBe(true);
    expect(isDaemonConnectionFailure(401)).toBe(false);
  });

  test("rotates a client token with the local Console session", async () => {
    let request: Request | undefined;
    globalThis.fetch = async (input, init) => {
      request = new Request(new URL(input.toString(), "http://localhost"), init);
      return Response.json({ client: { id: "cli_one", name: "One" }, token: "token" });
    };

    await expect(rotateClientToken("cli_one")).resolves.toMatchObject({ token: "token" });
    expect(request).toMatchObject({ method: "POST", url: "http://localhost/clients/cli_one/token" });
  });
});
