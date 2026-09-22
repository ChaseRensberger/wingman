import { describe, expect, test } from "bun:test";

import { buildModelRef, modelCallAnswerSpeed, splitModelRef } from "./utils";
import type { ModelCall } from "./types";

describe("model refs", () => {
  test("parses model IDs with slashes and a variant", () => {
    expect(splitModelRef("openrouter/anthropic/claude#high")).toEqual({
      provider: "openrouter",
      model: "anthropic/claude",
      variant: "high",
    });
  });

  test("builds default and named variant refs", () => {
    expect(buildModelRef("openai", "gpt-5.6-terra", null)).toBe("openai/gpt-5.6-terra");
    expect(buildModelRef("openai", "gpt-5.6-terra", "high")).toBe("openai/gpt-5.6-terra#high");
  });
});

test("answer speed excludes reasoning tokens generated before the first answer", () => {
  const call = {
    output_tokens: 1010,
    reasoning_tokens: 1000,
    started_at: "2026-09-20T12:00:00.000Z",
    completed_at: "2026-09-20T12:00:11.000Z",
    trace: { timing: { first_answer_ms: 10000 } },
  } as ModelCall;
  expect(modelCallAnswerSpeed(call)).toBe("10.0");
  expect(modelCallAnswerSpeed({ ...call, reasoning_tokens: 1010 })).toBeUndefined();
  expect(modelCallAnswerSpeed({ ...call, trace: undefined })).toBeUndefined();
});
