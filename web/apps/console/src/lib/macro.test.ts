import { expect, test } from "bun:test";

import { macroInvocation } from "./macro";

const macros = [{ id: "review", description: "Review a change" }];

test("macroInvocation separates a macro ID from natural language arguments", () => {
  expect(macroInvocation("/review Check auth changes and missing tests.", macros)).toEqual({
    macroID: "review",
    arguments: "Check auth changes and missing tests.",
  });
});

test("macroInvocation keeps unknown slash text as a normal message", () => {
  expect(macroInvocation("/model", macros)).toBeUndefined();
});
