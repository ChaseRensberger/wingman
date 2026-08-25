import { describe, expect, test } from "bun:test";

import { collapseOutput, parseUnifiedDiff, patchFiles, stripAnsi } from "./tool-display";

describe("tool output display", () => {
  test("collapses long output without hiding short output", () => {
    expect(collapseOutput("one\ntwo", 3, 100)).toEqual({ output: "one\ntwo", overflow: false });
    expect(collapseOutput("one\ntwo\nthree\nfour", 3, 100)).toEqual({
      output: "one\ntwo\nthree\n…",
      overflow: true,
    });
  });

  test("removes terminal color escapes", () => {
    expect(stripAnsi("\u001b[31mfailed\u001b[0m")).toBe("failed");
  });
});

describe("unified diff display", () => {
  test("tracks old and new line numbers across a hunk", () => {
    const lines = parseUnifiedDiff(
      [
        "--- a/example.go",
        "+++ b/example.go",
        "@@ -2,3 +2,4 @@",
        " context",
        "-old",
        "+new",
        "+added",
        " tail",
      ].join("\n"),
    );

    expect(lines.filter((line) => line.kind !== "header" && line.kind !== "hunk")).toEqual([
      { kind: "context", text: "context", oldLine: 2, newLine: 2 },
      { kind: "deletion", text: "old", oldLine: 3 },
      { kind: "addition", text: "new", newLine: 3 },
      { kind: "addition", text: "added", newLine: 4 },
      { kind: "context", text: "tail", oldLine: 4, newLine: 5 },
    ]);
  });

  test("reads file mutation metadata", () => {
    expect(
      patchFiles([
        {
          relativePath: "a.go",
          movePath: "b.go",
          type: "move",
          patch: "@@ -1 +1 @@",
          additions: 1,
          deletions: 1,
        },
      ]),
    ).toEqual([
      {
        filePath: undefined,
        relativePath: "a.go",
        movePath: "b.go",
        type: "move",
        patch: "@@ -1 +1 @@",
        additions: 1,
        deletions: 1,
      },
    ]);
  });

  test("does not mistake changed content for file headers", () => {
    const lines = parseUnifiedDiff("--- a/file\n+++ b/file\n@@ -1 +1 @@\n---old\n+++new\n");
    expect(lines.find((line) => line.kind === "deletion")).toEqual({
      kind: "deletion",
      text: "--old",
      oldLine: 1,
    });
    expect(lines.find((line) => line.kind === "addition")).toEqual({
      kind: "addition",
      text: "++new",
      newLine: 1,
    });
  });
});
