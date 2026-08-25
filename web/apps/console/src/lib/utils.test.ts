import { describe, expect, test } from "bun:test";

import { buildModelRef, splitModelRef } from "./utils";

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
