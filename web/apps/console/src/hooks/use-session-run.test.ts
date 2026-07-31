import { describe, expect, test } from "bun:test";

import {
	isTerminalSessionRunEvent,
	latestActiveSessionRun,
	sessionRunEventError,
	sessionRunRetryDelay,
	sessionStreamControl,
	terminalSessionRunError,
} from "./use-session-run";
import type { SessionRun } from "@/lib/types";

function run(id: string, sequence: number, status: SessionRun["status"]): SessionRun {
	return {
		id,
		session_id: "ses-1",
		admitted_version: sequence,
		sequence,
		status,
		message: "hello",
		agent: { id: "agt-1", name: "Agent", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
		created_at: "2026-01-01T00:00:00Z",
		updated_at: "2026-01-01T00:00:00Z",
	};
}

describe("session run recovery", () => {
	test("selects the latest queued or running run", () => {
		expect(latestActiveSessionRun([
			run("completed", 1, "completed"),
			run("queued", 2, "queued"),
			run("running", 3, "running"),
			run("failed", 4, "failed"),
		])).toMatchObject({ id: "running" });
	});
});

describe("session run terminal events", () => {
	test("recognizes every terminal status and uses server failure fields", () => {
		for (const type of ["session.run.completed", "session.run.failed", "session.run.aborted"]) {
			expect(isTerminalSessionRunEvent(type)).toBe(true);
		}
		expect(isTerminalSessionRunEvent("session.run.started")).toBe(false);
		expect(sessionRunEventError({ error_type: "run_failed", error_message: "provider unavailable", error: "legacy" })).toBe("provider unavailable");
		expect(sessionRunEventError({ error_type: "run_aborted" })).toBe("run_aborted");
	});

	test("uses authoritative terminal status and error fields", () => {
		expect(terminalSessionRunError(run("active", 1, "running"))).toBeUndefined();
		expect(terminalSessionRunError({ ...run("failed", 2, "failed"), error_type: "provider_error", error_message: "provider unavailable" })).toBe("provider unavailable");
		expect(terminalSessionRunError({ ...run("aborted", 3, "aborted"), error_type: "cancelled" })).toBe("cancelled");
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
});
