import { describe, expect, test } from "bun:test";

import { APIError, isDaemonConnectionFailure } from "./client";

describe("daemon connection failures", () => {
  test("keeps SDK API errors available to Console callers", () => {
    const error = new APIError(400, "invalid_request", "invalid request", "req-1");
    expect(error).toMatchObject({ status: 400, code: "invalid_request", requestId: "req-1" });
  });

  test("treats structured and plain server failures as daemon outages", () => {
    expect(isDaemonConnectionFailure(503)).toBe(true);
    expect(isDaemonConnectionFailure(500)).toBe(true);
    expect(isDaemonConnectionFailure(401)).toBe(false);
  });
});
