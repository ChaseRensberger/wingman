import { describe, expect, test } from "bun:test";

import {
  formatSessionError,
  modelRefExists,
  reasoningSummary,
  shouldAutoGenerateTitle,
  shouldShowThinking,
  titleSourceMessage,
  withFailedUserMessage,
} from "./session-detail";
import type { Session } from "./types";

test("an untitled session uses its first user message on a later turn", () => {
  const session = {
    id: "ses_untitled",
    title: "New session",
    history: [{ role: "user", content: [{ type: "text", text: "What is the current weather?" }] }],
  } as Session;
  expect(shouldAutoGenerateTitle(session)).toBe(true);
  expect(titleSourceMessage(session, "A follow-up question")).toBe("What is the current weather?");
  expect(shouldAutoGenerateTitle({ ...session, title: "Weather in New York" })).toBe(false);
});

describe("reasoningSummary", () => {
  test("extracts a provider reasoning-summary heading", () => {
    expect(
      reasoningSummary("**Inspecting client identity**\n\nChecking persisted client attribution."),
    ).toEqual({
      title: "Inspecting client identity",
      body: "Checking persisted client attribution.",
    });
  });

  test("keeps reasoning without a heading as the body", () => {
    expect(reasoningSummary("Checking persisted client attribution.")).toEqual({
      title: "",
      body: "Checking persisted client attribution.",
    });
  });
});

describe("shouldShowThinking", () => {
  test("only shows the generic status before visible activity", () => {
    expect(shouldShowThinking(true, false)).toBe(true);
    expect(shouldShowThinking(true, true)).toBe(false);
    expect(shouldShowThinking(false, false)).toBe(false);
  });
});

describe("withFailedUserMessage", () => {
  test("retains a rejected submission in an empty transcript", () => {
    expect(withFailedUserMessage([], "hello")).toEqual([
      { role: "user", content: [{ type: "text", text: "hello" }] },
    ]);
  });

  test("does not duplicate a persisted user message", () => {
    const messages = [
      { role: "user" as const, content: [{ type: "text" as const, text: "hello" }] },
    ];
    expect(withFailedUserMessage(messages, "hello")).toBe(messages);
  });
});

test("formatSessionError explains how to fix a missing working directory", () => {
  expect(
    formatSessionError(
      new Error(
        'session cannot start: tool "read" requires a working directory, but session has none',
      ),
    ),
  ).toBe(
    "This session has no working directory. Set its working directory before using this agent.",
  );
});

test("modelRefExists accepts only advertised variants", () => {
  const models = {
    openai: [{ id: "gpt-5.6-terra", variants: ["low", "high"] }],
  } as Parameters<typeof modelRefExists>[0];
  expect(modelRefExists(models, "openai/gpt-5.6-terra")).toBe(true);
  expect(modelRefExists(models, "openai/gpt-5.6-terra#high")).toBe(true);
  expect(modelRefExists(models, "openai/gpt-5.6-terra#max")).toBe(false);
});
