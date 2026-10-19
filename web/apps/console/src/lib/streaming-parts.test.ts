import { expect, test } from "bun:test";
import type { SessionEvent } from "@wingman-actor/client";

import { reduceStreamingParts, type StreamingPart } from "./streaming-parts";
import { reduceToolActivity } from "./tool-activity-state";
import { projectTranscript } from "./transcript";
import type { Message, ToolActivity } from "./types";

function event(type: SessionEvent["type"], data: Record<string, unknown>): SessionEvent {
  return { type, data: { run_id: "run", message_id: "message", ...data } } as SessionEvent;
}

test.each(["write", "read"])(
  "%s keeps commentary and interleaved text in place from deltas through persistence",
  (tool) => {
    let parts: StreamingPart[] = [];
    let activities = new Map<string, ToolActivity>();
    let messages: Message[] = [];
    const receive = (ev: SessionEvent) => {
      parts = reduceStreamingParts(parts, ev);
      if (ev.type === "session.tool.updated") {
        activities = reduceToolActivity(activities, { type: "updated", ...ev.data });
      }
      if (ev.type === "session.message.created") messages = [ev.data.message as Message];
    };
    const project = (visibleText?: string) =>
      projectTranscript({
        messages,
        toolCallsById: new Map(),
        toolResultsById: new Map(),
        toolActivitiesById: activities,
        streamingParts: parts,
        streamingText:
          visibleText ??
          parts.flatMap((part) => (part.type === "text" ? [part.text] : [])).join(""),
        isStreaming: true,
      });
    const contents = () =>
      project().flatMap((row) => {
        if (row.type === "message")
          return row.message.content.map((part) =>
            part.type === "text"
              ? part.text
              : part.type === "reasoning"
                ? part.reasoning
                : part.type,
          );
        if (row.type === "tool") return ["tool"];
        if (row.type === "exploration") return row.tools.map(() => "tool");
        return [];
      });

    receive(event("session.reasoning.delta", { part_id: "reasoning", delta: "Considering tabs" }));
    receive(event("session.text.delta", { part_id: "before", delta: "I will add " }));
    receive(event("session.text.delta", { part_id: "before", delta: "a picker." }));
    receive(
      event("session.tool.updated", {
        part_id: "tool-1",
        call_id: "call-1",
        tool,
        status: "pending",
      }),
    );
    expect(contents()).toEqual(["Considering tabs", "I will add a picker.", "tool"]);
    const firstToolKey = project().find(
      (row) => row.type === "tool" || row.type === "exploration",
    )?.key;
    receive(
      event("session.tool.updated", {
        part_id: "tool-1",
        call_id: "call-1",
        tool,
        status: "pending",
        input: { filePath: "shop.ts" },
      }),
    );
    expect(project().find((row) => row.type === "tool" || row.type === "exploration")?.key).toBe(
      firstToolKey,
    );
    receive(event("session.text.delta", { part_id: "after", delta: "Then update orders." }));
    receive(
      event("session.tool.updated", {
        part_id: "tool-2",
        call_id: "call-2",
        tool,
        status: "pending",
      }),
    );
    const expected = [
      "Considering tabs",
      "I will add a picker.",
      "tool",
      "Then update orders.",
      "tool",
    ];
    expect(contents()).toEqual(expected);
    const revealed = project("I will add a picker.Then");
    expect(
      revealed
        .filter((row) => row.type === "message")
        .flatMap((row) =>
          row.message.content.filter((part) => part.type === "text").map((part) => part.text),
        ),
    ).toEqual(["I will add a picker.", "Then"]);

    receive(
      event("session.reasoning.completed", { part_id: "reasoning", text: "Considering tabs" }),
    );
    receive(event("session.text.completed", { part_id: "before", text: "I will add a picker." }));
    expect(contents()).toEqual(expected);
    receive(
      event("session.message.created", {
        message: {
          id: "message",
          role: "assistant",
          content: [
            { type: "reasoning", reasoning: "Considering tabs" },
            { type: "text", text: "I will add a picker." },
            {
              id: "tool-1",
              type: "tool",
              name: tool,
              call_id: "call-1",
              tool_use_id: "use-1",
              state: "pending",
              input: { filePath: "shop.ts" },
            },
            { type: "text", text: "Then update orders." },
            {
              id: "tool-2",
              type: "tool",
              name: tool,
              call_id: "call-2",
              tool_use_id: "use-2",
              state: "pending",
              input: {},
            },
          ],
        },
      }),
    );
    expect(contents()).toEqual(expected);
    for (const n of [1, 2])
      receive(
        event("session.tool.updated", {
          call_id: `call-${n}`,
          tool_use_id: `use-${n}`,
          tool,
          status: "completed",
        }),
      );
    expect(parts).toEqual([]);
    expect(contents()).toEqual(expected);
  },
);

test("saving another message does not discard the current live parts", () => {
  let parts = reduceStreamingParts(
    [],
    event("session.text.delta", { part_id: "text", delta: "Current response" }),
  );
  parts = reduceStreamingParts(
    parts,
    event("session.message.created", { message: { id: "older", role: "assistant", content: [] } }),
  );
  expect(parts).toHaveLength(1);
  expect(
    reduceStreamingParts(
      parts,
      event("session.message.created", {
        message: { id: "message", role: "assistant", content: [] },
      }),
    ),
  ).toEqual([]);
});
