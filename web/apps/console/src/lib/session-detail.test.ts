import { describe, expect, test } from "bun:test";

import { reasoningSummary, shouldShowThinking } from "./session-detail";

describe("reasoningSummary", () => {
	test("extracts a provider reasoning-summary heading", () => {
		expect(reasoningSummary("**Inspecting client identity**\n\nChecking persisted client attribution.")).toEqual({
			title: "Inspecting client identity",
			body: "Checking persisted client attribution.",
		});
	});

	test("keeps reasoning without a heading as the body", () => {
		expect(reasoningSummary("Checking persisted client attribution.")).toEqual({ title: "", body: "Checking persisted client attribution." });
	});
});

describe("shouldShowThinking", () => {
	test("only shows the generic status before visible activity", () => {
		expect(shouldShowThinking(true, false)).toBe(true);
		expect(shouldShowThinking(true, true)).toBe(false);
		expect(shouldShowThinking(false, false)).toBe(false);
	});
});
