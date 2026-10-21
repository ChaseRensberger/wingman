import { expect, test } from "bun:test";
import { sessionQueue, withStartedRunMessages, type SessionNotice } from "./session-queue";
import { projectTranscript } from "./transcript";
import type { Agent, Message, SessionRun } from "./types";

const plan: Agent = {
  id: "plan",
  name: "Plan",
  model_ref: "test/one",
  created_at: "",
  updated_at: "",
};
const build: Agent = { ...plan, id: "build", name: "Build", model_ref: "test/two" };
const run = (
  id: string,
  sequence: number,
  status: SessionRun["status"],
  agent: Agent,
): SessionRun => ({
  id,
  sequence,
  status,
  agent,
  session_id: "s",
  admitted_version: sequence,
  message: id,
  created_at: "",
  updated_at: "",
  started_at: status === "queued" || status === "aborted" ? undefined : "2026-10-20T00:00:00Z",
});

test("queued previews compare consecutive turns and ignore cancelled unstarted turns", () => {
  const runs = [
    run("third", 3, "queued", plan),
    run("first", 1, "running", plan),
    run("second", 2, "queued", build),
  ];
  const queue = sessionQueue(runs);
  expect(queue.map(({ run }) => run.id)).toEqual(["second", "third"]);
  expect(queue[0]!.changes).toEqual(["Agent: Plan → Build", "Model: test/one → test/two"]);
  expect(queue[1]!.changes).toEqual(["Agent: Build → Plan", "Model: test/two → test/one"]);
  expect(
    sessionQueue([
      run("first", 1, "completed", plan),
      run("cancelled", 2, "aborted", build),
      run("next", 3, "queued", plan),
    ])[0]!.changes,
  ).toEqual([]);
  expect(runs[0]!.id).toBe("third");
});

test("saved switch events appear once before the matching message even with identical text", () => {
  const notice: SessionNotice = {
    id: "n",
    schema_version: 1,
    type: "session.agent.switched",
    cursor: { session_id: "s", seq: 5 },
    data: {
      run_id: "second",
      agent: "build",
      name: "Build",
      previous: "plan",
      previous_name: "Plan",
    },
  };
  const rows = projectTranscript({
    messages: [
      {
        id: "m1",
        role: "user",
        metadata: { run_id: "first" },
        content: [{ type: "text", text: "same" }],
      },
      {
        id: "m2",
        role: "user",
        metadata: { run_id: "second" },
        content: [{ type: "text", text: "same" }],
      },
      {
        id: "m3",
        role: "assistant",
        metadata: { run_id: "second" },
        content: [{ type: "text", text: "answer" }],
      },
    ],
    notices: [notice],
    runs: [run("first", 1, "completed", plan), run("second", 2, "running", build)],
    toolCallsById: new Map(),
    toolResultsById: new Map(),
    toolActivitiesById: new Map(),
  });
  expect(rows.map((row) => row.type)).toEqual(["message", "notice", "message", "message"]);
  expect(rows[1]).toMatchObject({ text: "Switched agent from Plan to Build" });
});

test.each(["running", "awaiting_input", "awaiting_completion"] as const)(
  "a %s prompt stays before its response until saved history arrives",
  (status) => {
    const active = { ...run("second", 2, status, build), message: "same" };
    const previous: Message = {
      id: "old",
      role: "user",
      metadata: { run_id: "first" },
      content: [{ type: "text", text: "same" }],
    };
    const response: Message = {
      id: "response",
      role: "assistant",
      metadata: { run_id: "second" },
      content: [{ type: "text", text: "answer" }],
    };
    const saved: Message = {
      id: "saved",
      role: "user",
      metadata: { run_id: "second" },
      content: [{ type: "text", text: "same" }],
    };
    const runs = [run("first", 1, "completed", plan), active];
    expect(
      withStartedRunMessages([previous], runs).map((message) => message.metadata?.run_id),
    ).toEqual(["first", "second"]);
    expect(
      withStartedRunMessages([previous, response], runs).map((message) => message.role),
    ).toEqual(["user", "user", "assistant"]);
    const history = [previous, saved, response];
    expect(withStartedRunMessages(history, runs)).toBe(history);
  },
);

test("queued messages and actions do not create transcript prompts", () => {
  expect(withStartedRunMessages([], [run("queued", 1, "queued", plan)])).toEqual([]);
  expect(
    withStartedRunMessages(
      [],
      [{ ...run("action", 1, "running", plan), kind: "action", action: "compaction.compact" }],
    ),
  ).toEqual([]);
});
