import { expect, test } from "bun:test";

import { actionInvocation } from "./action";

const actions = [
  { id: "compaction.compact", command: "compact", description: "Compact session context" },
];

test("actionInvocation resolves a slash command to its plugin action", () => {
  expect(actionInvocation("/compact", actions)).toEqual({
    action: "compaction.compact",
    arguments: "",
  });
});

test("actionInvocation preserves action arguments", () => {
  expect(actionInvocation("/compact older messages", actions)).toEqual({
    action: "compaction.compact",
    arguments: "older messages",
  });
});

test("actionInvocation ignores unknown slash commands", () => {
  expect(actionInvocation("/review", actions)).toBeUndefined();
});
