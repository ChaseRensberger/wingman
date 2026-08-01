import { describe, expect, test } from "bun:test";

import { APIError, apiData } from "./client";

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
});
