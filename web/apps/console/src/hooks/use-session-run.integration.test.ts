import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { act, createElement, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { ParsedSessionEvent, SessionEvent } from "@wingman-actor/client";

import { client } from "@/lib/client";
import { withStartedRunMessages } from "@/lib/session-queue";
import type { Session, SessionRun } from "@/lib/types";
import { useSessionRun } from "./use-session-run";

test("queue submissions preserve streaming, failures preserve the active turn, and Stop targets that turn", async () => {
  const window = new Window();
  const globals = { window, document: window.document, IS_REACT_ACT_ENVIRONMENT: true };
  const previous = new Map(
    Object.keys(globals).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]),
  );
  for (const [key, value] of Object.entries(globals))
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
  const container = window.document.createElement("div");
  window.document.body.append(container);
  const root = createRoot(container as unknown as HTMLElement);
  const original = {
    list: client.sessions.runs.list,
    get: client.sessions.runs.get,
    abort: client.sessions.runs.abort,
    listEvents: client.sessions.listEvents,
    streamEvents: client.sessions.streamEvents,
    permissions: client.sessions.permissionRequests.list,
    questions: client.sessions.questions.list,
  };
  const makeRun = (id: string, sequence: number, status: SessionRun["status"]): SessionRun => ({
    id,
    session_id: "s",
    sequence,
    admitted_version: sequence,
    status,
    message: id,
    agent: {
      id: sequence === 1 ? "plan" : "build",
      name: sequence === 1 ? "Plan" : "Build",
      model_ref: "test/model",
      created_at: "",
      updated_at: "",
    },
    created_at: "",
    updated_at: "",
    started_at: status === "running" ? "2026-10-20" : undefined,
  });
  const records = [makeRun("first", 1, "running")];
  const listeners = new Set<(event: ParsedSessionEvent) => void>();
  let seq = 0;
  const emit = (event: SessionEvent) => {
    const parsed: ParsedSessionEvent = {
      known: true,
      event: { ...event, id: `e${++seq}`, cursor: { session_id: "s", seq } },
    };
    for (const listener of listeners) listener(parsed);
  };
  client.sessions.runs.list = (async () => structuredClone(records)) as typeof original.list;
  client.sessions.runs.get = (async (_id, runID) =>
    structuredClone(records.find((run) => run.id === runID)!)) as typeof original.get;
  client.sessions.listEvents = async () => ({ data: [], has_more: false });
  client.sessions.permissionRequests.list = async () => [];
  client.sessions.questions.list = async () => [];
  client.sessions.streamEvents = async function* (_id, options) {
    const queue: ParsedSessionEvent[] = [];
    let wake: (() => void) | undefined;
    const listener = (event: ParsedSessionEvent) => {
      queue.push(event);
      wake?.();
    };
    const abort = () => wake?.();
    listeners.add(listener);
    options?.signal?.addEventListener("abort", abort);
    try {
      while (!options?.signal?.aborted) {
        if (!queue.length)
          await new Promise<void>((resolve) => {
            wake = resolve;
          });
        while (queue.length && !options?.signal?.aborted) yield queue.shift()!;
      }
    } finally {
      listeners.delete(listener);
      options?.signal?.removeEventListener("abort", abort);
    }
  };
  const stopped: string[] = [];
  client.sessions.runs.abort = (async (_id, runID) => {
    stopped.push(runID);
    records.find((run) => run.id === runID)!.status = "aborted";
    emit({ id: "", schema_version: 1, type: "session.run.aborted", data: { run_id: runID } });
  }) as typeof original.abort;
  let current!: ReturnType<typeof useSessionRun>;
  let session: Session | null = null;
  const loadSession = async () => {};
  function Harness() {
    const [state, setSession] = useState<Session | null>({
      id: "s",
      version: 1,
      history: [],
      created_at: "",
      updated_at: "",
    });
    const run = useSessionRun({ sessionId: "s", loadSession, setSession });
    useEffect(() => {
      current = run;
      session = state;
    });
    return null;
  }
  const flush = () =>
    act(async () => {
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
    });
  const text = (runID: string, delta: string): SessionEvent => ({
    id: "",
    schema_version: 1,
    type: "session.text.delta",
    data: { run_id: runID, delta },
  });
  try {
    await act(async () => root.render(createElement(Harness)));
    await flush();
    await act(async () => emit(text("first", "Working")));
    expect(current.streamingText).toBe("Working");
    await act(async () =>
      emit({
        id: "",
        schema_version: 1,
        type: "session.agent.switched",
        data: { run_id: "first", agent: "plan", name: "Plan" },
      }),
    );
    expect(session?.notices).toHaveLength(1);
    expect(session?.notices?.[0]?.type).toBe("session.agent.switched");

    await act(async () => {
      expect(
        current.begin({ message: "second", agentId: "build", modelRef: "test/model" }),
      ).not.toBeNull();
      expect(
        current.begin({ message: "duplicate", agentId: "build", modelRef: "test/model" }),
      ).toBeNull();
      records.push(makeRun("second", 2, "queued"));
      await current.accept("s", "second");
      current.finishSubmission();
    });
    await flush();
    await act(async () => emit(text("first", " still")));
    expect(current.streamingText).toBe("Working still");
    expect(current.runs.map((run) => run.id)).toEqual(["first", "second"]);

    await act(async () => {
      current.begin({ message: "third", agentId: "build", modelRef: "test/model" });
      await current.fail(new Error("Admission unavailable"), "s");
      current.finishSubmission();
    });
    await flush();
    await act(async () => emit(text("first", " streaming")));
    expect(current.streamingText).toBe("Working still streaming");
    expect(current.failedRun?.message).toBe("third");
    expect(current.isStreaming).toBe(true);

    await act(async () => {
      current.begin({ message: "third", agentId: "build", modelRef: "test/model" });
      records.push(makeRun("third", 3, "queued"));
      await current.accept("s", "third");
      current.finishSubmission();
    });
    await flush();
    await act(async () => root.render(null));
    await act(async () => root.render(createElement(Harness)));
    await flush();
    expect(current.runs.map((run) => run.id)).toEqual(["first", "second", "third"]);
    await act(async () => emit(text("first", "Recovered current turn")));
    expect(current.streamingText).toBe("Recovered current turn");

    await act(async () => current.abort("s"));
    await flush();
    expect(stopped).toEqual(["first"]);
    expect(records[1]!.status).toBe("queued");
    await act(async () => {
      records[1]!.status = "running";
      emit({ id: "", schema_version: 1, type: "session.run.started", data: { run_id: "second" } });
      emit(text("second", "Next turn"));
    });
    expect(current.streamingText).toBe("Next turn");
    await flush();
    expect(withStartedRunMessages([], current.runs)).toMatchObject([
      { role: "user", metadata: { run_id: "second" }, content: [{ text: "second" }] },
    ]);
    await act(async () => {
      records[1]!.status = "completed";
      emit({
        id: "",
        schema_version: 1,
        type: "session.run.completed",
        data: { run_id: "second" },
      });
    });
    await flush();
    await act(async () => emit(text("third", "Third turn")));
    expect(current.streamingText).toBe("Third turn");

    await act(async () => {
      records[2]!.status = "completed";
      emit({ id: "", schema_version: 1, type: "session.run.completed", data: { run_id: "third" } });
    });
    await flush();
    await act(async () => {
      current.begin({ message: "fast failure", agentId: "build", modelRef: "test/model" });
      records.push({
        ...makeRun("fast", 4, "failed"),
        message: "fast failure",
        error_message: "Provider unavailable",
      });
      await current.accept("s", "fast");
      current.finishSubmission();
    });
    await flush();
    expect(current.isStreaming).toBe(false);
    expect(current.failedRun).toMatchObject({
      message: "fast failure",
      error: "Provider unavailable",
    });
    await act(async () =>
      emit({
        id: "",
        schema_version: 1,
        type: "session.model.switched",
        data: { run_id: "fast", model_ref: "test/other" },
      }),
    );
    expect(session?.notices).toHaveLength(1);
    expect(session?.notices?.[0]?.type).toBe("session.model.switched");

    const actionInput = { arguments: "keep recent changes", limit: 10 };
    await act(async () => {
      current.begin({
        message: "/compact keep recent changes",
        agentId: "build",
        modelRef: "test/model",
        kind: "action",
      });
      records.push({
        ...makeRun("action", 5, "running"),
        kind: "action",
        message: "",
        action: "compaction.compact",
        input: actionInput,
      });
      await current.accept("s", "action");
      current.finishSubmission();
    });
    await flush();
    await act(async () => root.render(null));
    await act(async () => root.render(createElement(Harness)));
    await flush();
    await act(async () => {
      records[4]!.status = "failed";
      emit({
        id: "",
        schema_version: 1,
        type: "session.run.failed",
        data: { run_id: "action", error_message: "Compaction failed" },
      });
    });
    await flush();
    expect(current.failedRun).toMatchObject({
      kind: "action",
      action: "compaction.compact",
      input: actionInput,
      error: "Compaction failed",
    });
  } finally {
    await act(async () => root.unmount());
    client.sessions.runs.list = original.list;
    client.sessions.runs.get = original.get;
    client.sessions.runs.abort = original.abort;
    client.sessions.listEvents = original.listEvents;
    client.sessions.streamEvents = original.streamEvents;
    client.sessions.permissionRequests.list = original.permissions;
    client.sessions.questions.list = original.questions;
    await window.happyDOM.close();
    for (const [key, descriptor] of previous) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
});
