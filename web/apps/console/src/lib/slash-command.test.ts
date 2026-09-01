import { expect, test } from "bun:test";

import { slashCommandQuery } from "./slash-command";

test("slashCommandQuery opens the command menu for a bare slash", () => {
  expect(slashCommandQuery("/")).toBe("");
});

test("slashCommandQuery only matches an unfinished command token", () => {
  expect(slashCommandQuery("/review")).toBe("review");
  expect(slashCommandQuery("/review authentication")).toBeUndefined();
});
