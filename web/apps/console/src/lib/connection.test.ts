import { describe, expect, test } from "bun:test";

import { daemonConnectionMessage, daemonFailurePhase, daemonRetryDelay } from "./connection";

describe("daemon connection recovery", () => {
  test("caps exponential retry delays", () => {
    expect(daemonRetryDelay(0)).toBe(250);
    expect(daemonRetryDelay(1)).toBe(500);
    expect(daemonRetryDelay(10)).toBe(5_000);
  });

  test("reports a persistent failure after five attempts", () => {
    expect(daemonFailurePhase(1)).toBe("retrying");
    expect(daemonFailurePhase(4)).toBe("retrying");
    expect(daemonFailurePhase(5)).toBe("failed");
  });

  test("describes every visible connection state", () => {
    expect(daemonConnectionMessage("connecting")).toContain("Connecting");
    expect(daemonConnectionMessage("retrying")).toContain("Reconnecting");
    expect(daemonConnectionMessage("paused")).toContain("paused");
    expect(daemonConnectionMessage("failed")).toContain("unavailable");
    expect(daemonConnectionMessage("live")).toContain("Connected");
  });
});
