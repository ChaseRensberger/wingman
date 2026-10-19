import { describe, expect, test } from "bun:test";

import {
  explorationSummary,
  projectTranscript,
  toolPartProps,
  transcriptToolStatus,
} from "./transcript";
import type { Message, Part, ToolActivity, ToolCallPart, ToolPart } from "./types";

function tool(name: string, id: string, state: ToolPart["state"] = "completed"): ToolPart {
  return {
    type: "tool",
    tool_use_id: id,
    call_id: `call-${id}`,
    name,
    state,
    input: { filePath: "src/index.ts" },
  };
}

function assistant(id: string, content: Part[]): Message {
  return { id, role: "assistant", content };
}

function project(
  messages: Message[],
  overrides: Partial<Parameters<typeof projectTranscript>[0]> = {},
) {
  return projectTranscript({
    messages,
    toolCallsById: new Map(),
    toolResultsById: new Map(),
    toolActivitiesById: new Map(),
    ...overrides,
  });
}

describe("exploration transcript", () => {
  test("groups searches and reads across assistant messages", () => {
    const rows = project([
      assistant("one", [tool("grep", "1"), tool("read", "2")]),
      assistant("two", [
        tool("glob", "3"),
        tool("websearch", "4"),
        tool("grep", "5"),
        tool("read", "6"),
        tool("read", "7"),
      ]),
    ]);
    expect(rows).toHaveLength(1);
    const row = rows[0];
    if (row.type !== "exploration") throw new Error("Expected exploration");
    expect(row.tools.map((item) => item.call.tool_use_id)).toEqual([
      "1",
      "2",
      "3",
      "4",
      "5",
      "6",
      "7",
    ]);
    expect(explorationSummary(row.tools)).toBe("Explored: 4 searches, 3 reads");
  });

  test.each([
    { type: "text", text: "Next step" },
    { type: "reasoning", reasoning: "Consider the result" },
    tool("bash", "bash"),
    tool("edit", "edit"),
    { type: "compaction_marker", summary: "Checkpoint" },
  ] as Part[])("keeps a %j boundary in order", (boundary) => {
    const rows = project([
      assistant("one", [tool("read", "1")]),
      assistant("two", [boundary]),
      assistant("three", [tool("grep", "2")]),
    ]);
    expect(rows.map((row) => row.type)).toEqual(["exploration", "message", "exploration"]);
    if (rows[1].type === "message") expect(rows[1].message.content).toEqual([boundary]);
  });

  test("never groups across a user message", () => {
    const rows = project([
      assistant("one", [tool("read", "1")]),
      { role: "user", content: [{ type: "text", text: "Continue" }] },
      assistant("two", [tool("read", "2")]),
    ]);
    expect(rows.map((row) => row.type)).toEqual(["exploration", "message", "exploration"]);
  });

  test("ignores empty text and reasoning and hidden tool results", () => {
    const rows = project(
      [
        assistant("one", [tool("read", "1")]),
        assistant("two", [
          { type: "text", text: " " },
          { type: "reasoning", reasoning: "" },
          { type: "tool_result", call_id: "old-call", output: [] },
          tool("grep", "2"),
        ]),
      ],
      {
        toolCallsById: new Map([
          ["old-call", { type: "tool_call", call_id: "old-call", name: "read", input: {} }],
        ]),
      },
    );
    expect(rows).toHaveLength(1);
    if (rows[0].type === "exploration") expect(rows[0].tools).toHaveLength(2);
  });

  test("splits mixed messages without losing text or repeating metadata", () => {
    const message = assistant("one", [
      { type: "text", text: "Before" },
      tool("read", "1"),
      { type: "text", text: "After" },
    ]);
    const rows = project([message]);
    expect(rows.map((row) => row.type)).toEqual(["message", "exploration", "message"]);
    const messages = rows.filter((row) => row.type === "message");
    expect(messages.map((row) => row.message.content)).toEqual([
      [message.content[0]],
      [message.content[2]],
    ]);
    expect(messages.map((row) => row.showMeta)).toEqual([false, true]);
    expect(messages[1].sourceMessage).toBe(message);
  });

  test("shows response metadata only after the final text in a turn", () => {
    const rows = project([
      assistant("thought", [{ type: "reasoning", reasoning: "Considering the request" }]),
      assistant("tool-step", [tool("grep", "1")]),
      assistant("answer", [{ type: "text", text: "Here is the answer" }]),
      { role: "user", content: [{ type: "text", text: "Next" }] },
      assistant("next-answer", [{ type: "text", text: "Another answer" }]),
    ]);
    expect(
      rows
        .filter((row) => row.type === "message" && row.message.role === "assistant")
        .map((row) => [row.message.id, row.showMeta]),
    ).toEqual([
      ["thought", false],
      ["answer", true],
      ["next-answer", true],
    ]);
  });

  test("defers current response metadata while streaming or tools are active", () => {
    const messages = [assistant("answer", [{ type: "text", text: "Still working" }])];
    for (const rows of [
      project(messages, { isStreaming: true }),
      project(messages, {
        toolActivitiesById: new Map([
          ["tool", { call_id: "tool", tool: "grep", status: "running" }],
        ]),
      }),
    ]) {
      expect(rows.find((row) => row.type === "message")?.showMeta).toBe(false);
    }
  });

  test("joins live activities to history and keeps the group key after persistence", () => {
    const activity: ToolActivity = {
      tool_use_id: "2",
      call_id: "call-2",
      tool: "grep",
      status: "running",
      input: { pattern: "test" },
    };
    const live = project([assistant("one", [tool("read", "1")])], {
      toolActivitiesById: new Map([["2", activity]]),
    });
    const saved = project(
      [assistant("one", [tool("read", "1")]), assistant("two", [tool("grep", "2")])],
      { toolActivitiesById: new Map([["2", activity]]) },
    );
    expect(live[0].key).toBe(saved[0].key);
    if (live[0].type !== "exploration" || saved[0].type !== "exploration")
      throw new Error("Expected exploration");
    expect(live[0].tools).toHaveLength(2);
    expect(explorationSummary(live[0].tools)).toBe("Exploring: 1 read, 1 search");
    expect(saved[0].tools).toHaveLength(2);
    expect(explorationSummary(saved[0].tools)).toBe("Explored: 1 read, 1 search");
  });

  test("keeps a live-only group key when its first call is saved", () => {
    const activity: ToolActivity = {
      tool_use_id: "1",
      call_id: "call-1",
      tool: "read",
      status: "running",
    };
    const live = project([], { toolActivitiesById: new Map([["1", activity]]) });
    const saved = project([assistant("one", [tool("read", "1")])]);
    expect(live[0].key).toBe(saved[0].key);
  });

  test("uses legacy results and live progress without rendering either twice", () => {
    const call: ToolCallPart = {
      type: "tool_call",
      tool_use_id: "1",
      call_id: "call-1",
      name: "grep",
      input: { pattern: "find" },
    };
    const result = {
      type: "tool_result" as const,
      tool_use_id: "1",
      call_id: "call-1",
      output: [],
      metadata: { matches: 20 },
    };
    const rows = project([assistant("one", [call]), assistant("two", [result])], {
      toolCallsById: new Map([["1", call]]),
      toolResultsById: new Map([["1", result]]),
      toolActivitiesById: new Map([
        ["1", { tool_use_id: "1", call_id: "call-1", tool: "grep", status: "completed" }],
      ]),
    });
    expect(rows).toHaveLength(1);
    if (rows[0].type !== "exploration") throw new Error("Expected exploration");
    expect(rows[0].tools).toHaveLength(1);
    expect(rows[0].tools[0].result).toBe(result);
    expect(transcriptToolStatus(rows[0].tools[0])).toBe("completed");
  });

  test("does not deduplicate different tool-use IDs with the same call ID", () => {
    const activity: ToolActivity = {
      tool_use_id: "2",
      call_id: "call-1",
      tool: "read",
      status: "running",
    };
    const rows = project([assistant("one", [tool("read", "1")])], {
      toolActivitiesById: new Map([["2", activity]]),
    });
    if (rows[0].type !== "exploration") throw new Error("Expected exploration");
    expect(rows[0].tools).toHaveLength(2);
  });

  test("live reasoning and text remain boundaries around live tool activity", () => {
    const rows = project([assistant("one", [tool("read", "1")])], {
      streamingReasoning: "Thinking",
      isReasoningStreaming: true,
      streamingText: "Answer",
      toolActivitiesById: new Map([
        ["2", { tool_use_id: "2", call_id: "call-2", tool: "grep", status: "running" }],
      ]),
    });
    expect(rows.map((row) => row.type)).toEqual([
      "exploration",
      "message",
      "exploration",
      "message",
    ]);
    expect(
      rows.filter((row) => row.type === "message").every((row) => row.isStreaming && !row.showMeta),
    ).toBe(true);
  });

  test("retained reasoning stops spinning when tool execution begins", () => {
    const rows = project([], {
      isStreaming: true,
      streamingReasoning: "I should search the source",
      isReasoningStreaming: false,
      toolActivitiesById: new Map([["1", { call_id: "1", tool: "grep", status: "running" }]]),
    });
    const reasoning = rows.find((row) => row.type === "message");
    expect(reasoning?.message.content).toEqual([
      { type: "reasoning", reasoning: "I should search the source" },
    ]);
    expect(reasoning?.isStreaming).toBe(false);
    expect(rows[1].type).toBe("exploration");
    if (rows[1].type !== "exploration") throw new Error("Expected exploration");
    expect(transcriptToolStatus(rows[1].tools[0])).toBe("running");
  });

  test("commands and unknown live tools stay outside exploration", () => {
    const rows = project([], {
      toolActivitiesById: new Map([
        ["1", { call_id: "1", tool: "bash", status: "running" }],
        ["2", { call_id: "2", tool: "custom", status: "pending" }],
      ]),
    });
    expect(rows.map((row) => row.type)).toEqual(["tool", "tool"]);
  });

  test("counts fetches and uses singular labels", () => {
    const rows = project([
      assistant("one", [
        tool("webfetch", "1"),
        tool("webfetch", "2"),
        tool("grep", "3"),
        tool("read", "4", "error"),
      ]),
    ]);
    if (rows[0].type !== "exploration") throw new Error("Expected exploration");
    expect(explorationSummary(rows[0].tools)).toBe("Explored: 2 fetches, 1 search, 1 read");
  });

  test("preserves tool output, images, metadata, and errors", () => {
    const part: ToolPart = {
      ...tool("read", "1", "error"),
      error: "Permission denied",
      output_parts: [{ type: "image", url: "https://example.com/image.png" }],
      structured: { detail: "result" },
      metadata: { matches: 2 },
    };
    const props = toolPartProps(part);
    expect(props.result?.output).toBe(part.output_parts);
    expect(props.result?.structured).toBe(part.structured);
    expect(props.result?.metadata).toBe(part.metadata);
    expect(props.activity?.error).toBe("Permission denied");
  });
});
