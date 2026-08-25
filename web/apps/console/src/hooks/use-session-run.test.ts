import { describe, expect, test } from "bun:test";

import {
  isTerminalSessionRunEvent,
  addPermissionReplyInFlight,
  latestActiveSessionRun,
  maintainSessionRunStream,
  pendingPermissionRequests,
  reconcileSessionEventSeq,
  reducePermissionRequestRecords,
  sessionRunEventError,
  sessionRunRetryDelay,
  sessionStreamControl,
  terminalSessionRunError,
} from "./use-session-run";
import type { PermissionRequest, SessionRun } from "@/lib/types";

function run(id: string, sequence: number, status: SessionRun["status"]): SessionRun {
  return {
    id,
    session_id: "ses-1",
    admitted_version: sequence,
    sequence,
    status,
    message: "hello",
    agent: {
      id: "agt-1",
      name: "Agent",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function permission(id: string, createdAt: string, runID = "run-1"): PermissionRequest {
  return {
    id,
    session_id: "ses-1",
    run_id: runID,
    action: "file.write",
    resources: ["/tmp/file"],
    status: "pending",
    created_at: createdAt,
    updated_at: createdAt,
  };
}

describe("session run recovery", () => {
  test("selects the latest queued or running run", () => {
    expect(
      latestActiveSessionRun([
        run("completed", 1, "completed"),
        run("queued", 2, "queued"),
        run("running", 3, "running"),
        run("failed", 4, "failed"),
      ]),
    ).toMatchObject({ id: "running" });
  });
});

describe("session run terminal events", () => {
  test("recognizes every terminal status and uses server failure fields", () => {
    for (const type of ["session.run.completed", "session.run.failed", "session.run.aborted"]) {
      expect(isTerminalSessionRunEvent(type)).toBe(true);
    }
    expect(isTerminalSessionRunEvent("session.run.started")).toBe(false);
    expect(
      sessionRunEventError({
        error_type: "run_failed",
        error_message: "provider unavailable",
        error: "legacy",
      }),
    ).toBe("provider unavailable");
    expect(sessionRunEventError({ error_type: "run_aborted" })).toBe("run_aborted");
  });

  test("uses authoritative terminal status and error fields", () => {
    expect(terminalSessionRunError(run("active", 1, "running"))).toBeUndefined();
    expect(
      terminalSessionRunError({
        ...run("failed", 2, "failed"),
        error_type: "provider_error",
        error_message: "provider unavailable",
      }),
    ).toBe("provider unavailable");
    expect(
      terminalSessionRunError({ ...run("aborted", 3, "aborted"), error_type: "cancelled" }),
    ).toBe("cancelled");
  });
});

describe("session stream reconnection", () => {
  test("caps exponential retry delays", () => {
    expect(sessionRunRetryDelay(0)).toBe(250);
    expect(sessionRunRetryDelay(1)).toBe(500);
    expect(sessionRunRetryDelay(10)).toBe(5_000);
  });

  test("classifies synchronization controls separately from run activity", () => {
    expect(sessionStreamControl("session.events.synchronized")).toBe("synchronized");
    expect(sessionStreamControl("session.events.resync_required")).toBe("resync_required");
    expect(sessionStreamControl("session.text.delta")).toBeUndefined();
  });

  test("resets an invalid cursor only after an explicit resync", () => {
    expect(reconcileSessionEventSeq(12, 8, false)).toBe(12);
    expect(reconcileSessionEventSeq(12, 8, true)).toBe(8);
  });

  test("reconnects an active run after an unavailable stream and reloads authoritative state", async () => {
    let current = true;
    let subscriptions = 0;
    let reloads = 0;
    const delays: number[] = [];
    await maintainSessionRunStream({
      isCurrent: () => current,
      isCompleted: () => false,
      subscribe: async () => {
        subscriptions++;
        if (subscriptions === 1) throw new Error("HTTP 503");
        current = false;
        return { resync: false, synchronized: false };
      },
      clearVolatileStreamState: () => {},
      reload: async () => {
        reloads++;
        return run("run-1", 1, "running");
      },
      finish: async () => {
        throw new Error("active run should not finish");
      },
      resync: async () => {},
      waitForRetry: async (delay) => {
        delays.push(delay);
      },
      reportFailure: () => {},
    });
    expect(subscriptions).toBe(2);
    expect(reloads).toBe(1);
    expect(delays).toEqual([250]);
  });

  test("finishes from an authoritative terminal run after an unauthorized stream", async () => {
    let current = true;
    let finished: string | undefined;
    await maintainSessionRunStream({
      isCurrent: () => current,
      isCompleted: () => false,
      subscribe: async () => {
        throw new Error("HTTP 401");
      },
      clearVolatileStreamState: () => {},
      reload: async () => ({ ...run("run-1", 1, "failed"), error_message: "daemon restarted" }),
      finish: async (error) => {
        finished = error;
        current = false;
      },
      resync: async () => {},
      waitForRetry: async () => {
        throw new Error("terminal run should not retry");
      },
      reportFailure: () => {},
    });
    expect(finished).toBe("daemon restarted");
  });
});

describe("pending permission requests", () => {
  test("upserts requested events idempotently and orders pending requests deterministically", () => {
    let records = reducePermissionRequestRecords(new Map(), {
      type: "requested",
      request: permission("second", "2026-01-02T00:00:00Z"),
    });
    records = reducePermissionRequestRecords(records, {
      type: "requested",
      request: permission("first", "2026-01-01T00:00:00Z"),
    });
    records = reducePermissionRequestRecords(records, {
      type: "requested",
      request: {
        ...permission("first", "2026-01-01T00:00:00Z"),
        action: "file.delete",
        updated_at: "2026-01-03T00:00:00Z",
      },
    });
    expect(pendingPermissionRequests(records).map((request) => request.id)).toEqual([
      "first",
      "second",
    ]);
    expect(records.get("first")?.action).toBe("file.delete");
  });

  test("keeps a resolved SSE request hidden when a stale pending snapshot arrives", () => {
    const pending = permission("per-1", "2026-01-01T00:00:00Z");
    let records = reducePermissionRequestRecords(new Map(), {
      type: "resolved",
      request: {
        ...pending,
        status: "approved",
        response: "once",
        updated_at: "2026-01-02T00:00:00Z",
      },
    });
    records = reducePermissionRequestRecords(records, { type: "loaded", requests: [pending] });
    expect(records.get("per-1")?.status).toBe("approved");
    expect(pendingPermissionRequests(records)).toEqual([]);
  });

  test("does not let a stale requested replay replace a resolved record", () => {
    const pending = permission("per-1", "2026-01-01T00:00:00Z");
    let records = reducePermissionRequestRecords(new Map(), {
      type: "resolved",
      request: {
        ...pending,
        status: "rejected",
        response: "reject",
        updated_at: "2026-01-02T00:00:00Z",
      },
    });
    records = reducePermissionRequestRecords(records, { type: "requested", request: pending });
    expect(records.get("per-1")?.status).toBe("rejected");
  });

  test("uses newer records and prefers terminal records on matching versions", () => {
    const initial = permission("per-1", "2026-01-01T00:00:00Z");
    let records = reducePermissionRequestRecords(new Map(), {
      type: "requested",
      request: initial,
    });
    records = reducePermissionRequestRecords(records, {
      type: "requested",
      request: { ...initial, action: "file.delete", updated_at: "2026-01-02T00:00:00Z" },
    });
    records = reducePermissionRequestRecords(records, {
      type: "resolved",
      request: {
        ...initial,
        status: "approved",
        response: "always",
        updated_at: "2026-01-02T00:00:00Z",
      },
    });
    expect(records.get("per-1")).toMatchObject({ status: "approved", response: "always" });
  });

  test("tracks response-in-flight requests without duplicate entries", () => {
    const once = addPermissionReplyInFlight(new Set(), "per-1");
    const twice = addPermissionReplyInFlight(once, "per-1");
    expect([...twice]).toEqual(["per-1"]);
  });
});
