import { describe, expect, test } from "bun:test";

import { isTerminalSessionRunEvent, latestActiveSessionRun, sessionRunEventError } from "./use-session-run";
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
	});
});
