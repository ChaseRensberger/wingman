import { describe, expect, test } from "bun:test";

import { lookupToolActivity, normalizeToolStatus, reduceToolActivity } from "./tool-activity-state";

describe("tool activity state", () => {
	test("migrates pre-proposal input to its durable tool use", () => {
		let activities = reduceToolActivity(new Map(), { type: "input", run_id: "run-1", call_id: "call-1", tool: "read", delta: '{"path":' });
		activities = reduceToolActivity(activities, { type: "updated", tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", tool: "read", status: "proposed" });

		expect(activities.get("tlu-1")).toMatchObject({ tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", input_text: '{"path":', status: "pending" });
		expect(activities.has("run-1:call-1")).toBe(false);
	});

	test("keeps repeated provider call IDs distinct across durable tool uses", () => {
		let activities = reduceToolActivity(new Map(), { type: "updated", tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", tool: "read", status: "started" });
		activities = reduceToolActivity(activities, { type: "updated", tool_use_id: "tlu-2", run_id: "run-2", call_id: "call-1", tool: "bash", status: "authorized" });

		expect(activities.size).toBe(2);
		expect(activities.get("tlu-1")).toMatchObject({ run_id: "run-1", tool: "read", status: "running" });
		expect(activities.get("tlu-2")).toMatchObject({ run_id: "run-2", tool: "bash", status: "pending" });
		expect(lookupToolActivity(activities, { tool_use_id: "tlu-missing", call_id: "call-1" })).toBeUndefined();
	});

	test("normalizes every server lifecycle status", () => {
		expect(normalizeToolStatus("proposed")).toBe("pending");
		expect(normalizeToolStatus("authorized")).toBe("pending");
		expect(normalizeToolStatus("pending")).toBe("pending");
		expect(normalizeToolStatus("started")).toBe("running");
		expect(normalizeToolStatus("running")).toBe("running");
		expect(normalizeToolStatus("completed")).toBe("completed");
		for (const status of ["failed", "interrupted", "declined", "error"]) expect(normalizeToolStatus(status)).toBe("error");
	});

	test("merges durable progress and terminal updates", () => {
		let activities = reduceToolActivity(new Map(), { type: "progress", tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", tool: "bash", output_delta: "first", metadata: { step: 1 } });
		activities = reduceToolActivity(activities, { type: "progress", tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", output_delta: " second", metadata: { total: 2 } });
		activities = reduceToolActivity(activities, { type: "updated", tool_use_id: "tlu-1", run_id: "run-1", call_id: "call-1", tool: "bash", status: "completed", completed_at: "now" });

		expect(activities.get("tlu-1")).toMatchObject({ status: "completed", output: "first second", metadata: { step: 1, total: 2 }, completed_at: "now" });
	});
});
