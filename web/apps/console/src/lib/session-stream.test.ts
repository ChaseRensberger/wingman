import { expect, test } from "bun:test";

import { parseRunStreamEvent, parseSessionEvent, readSSE } from "@wingman-actor/client";
import { generatedSessionTitle } from "./session-stream";

test("session titles use a short topic instead of a model's answer", () => {
  const message = "What is the current weather in New York City?";
  expect(generatedSessionTitle("New York weather", message)).toBe("New York weather");
  expect(generatedSessionTitle("I don't have live internet access or real-time data feeds, so I can't help", message)).toBe(
    "Current weather in New York City",
  );
  expect(generatedSessionTitle("", message)).toBe("Current weather in New York City");
  expect(generatedSessionTitle("A very long generated answer that goes far beyond seven words", message)).toBe(
    "Current weather in New York City",
  );
});

test("readSSE preserves SSE event ids", async () => {
  const response = new Response('id:42\nevent: session.text.delta\ndata: {"delta":"hello"}\n\n');
  const events = await Array.fromAsync(readSSE(response));

  expect(events).toEqual([{ id: "42", event: "session.text.delta", data: { delta: "hello" } }]);
});

test("parseSessionEvent discriminates known and unknown payloads", () => {
  const known = parseSessionEvent({
    id: "evt-1",
    type: "session.text.delta",
    schema_version: 1,
    data: { run_id: "run-1", delta: "hi" },
  });
  if (!known?.known) throw new Error("known event was not classified");
  expect(known.event.type).toBe("session.text.delta");
  expect(known.event.data).toEqual({ run_id: "run-1", delta: "hi" });

  const unknown = parseSessionEvent({
    id: "evt-2",
    type: "plugin.custom",
    schema_version: 1,
    data: { value: 1 },
  });
  expect(unknown).toEqual({
    known: false,
    event: { id: "evt-2", type: "plugin.custom", schema_version: 1, data: { value: 1 } },
  });
  expect(
    parseSessionEvent({ type: "session.text.delta", schema_version: 1, data: {} }),
  ).toBeUndefined();
  expect(
    parseSessionEvent({ id: "evt-3", type: "session.text.delta", schema_version: 1, data: null }),
  ).toBeUndefined();
  expect(
    parseSessionEvent({
      id: "evt-4",
      type: "session.text.delta",
      data: { run_id: "run-1", delta: "hi" },
    }),
  ).toBeUndefined();
  expect(
    parseSessionEvent({
      id: "evt-5",
      type: "session.text.delta",
      schema_version: 2,
      data: { run_id: "run-1", delta: "hi" },
    }),
  ).toBeUndefined();
});

test("parseRunStreamEvent discriminates known and unknown payloads", () => {
  const known = parseRunStreamEvent({
    type: "done",
    version: 1,
    data: { usage: { input_tokens: 1, output_tokens: 2, total_tokens: 3 }, steps: 1 },
  });
  if (!known?.known) throw new Error("known event was not classified");
  expect(known.event.type).toBe("done");

  expect(parseRunStreamEvent({ type: "extension", version: 1, data: { value: true } })).toEqual({
    known: false,
    event: { type: "extension", version: 1, data: { value: true } },
  });
  expect(parseRunStreamEvent({ type: "done", data: {} })).toBeUndefined();
  expect(parseRunStreamEvent({ type: "done", version: 1, data: null })).toBeUndefined();
});
