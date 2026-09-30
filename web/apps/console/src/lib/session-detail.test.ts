import { describe, expect, test } from "bun:test";

import {
  formatSessionError,
  loadSessionTools,
  modelRefExists,
  reasoningSummary,
  sessionAgents,
  selectSessionAgent,
  shouldAutoGenerateTitle,
  shouldShowThinking,
  titleSourceMessage,
  withFailedUserMessage,
} from "./session-detail";
import type { Agent, Session, ToolCatalogItem } from "./types";

describe("session agent selection", () => {
  const agents: Agent[] = [
    { id: "code", name: "Code", tools: ["read"], created_at: "", updated_at: "" },
    { id: "chat", name: "Chat", created_at: "", updated_at: "" },
    { id: "search", name: "Search", tools: ["web_search"], created_at: "", updated_at: "" },
    { id: "plugin", name: "Plugin", tools: ["custom"], created_at: "", updated_at: "" },
  ];
  const tools: ToolCatalogItem[] = [
    { name: "read", source: "native", directory_scoped: true },
    { name: "web_search", source: "native" },
    { name: "custom", source: "plugin", directory_scoped: true },
  ];

  test("hides agents with directory-scoped tools when no directory is set", () => {
    expect(sessionAgents(agents, tools)).toEqual([agents[1], agents[2]]);
    expect(sessionAgents(agents, tools, "")).toEqual([agents[1], agents[2]]);
    expect(sessionAgents([{ ...agents[0]!, tools: ["web_search", "read"] }], tools)).toEqual([]);
  });

  test("shows all agents with a session or draft workspace directory", () => {
    expect(sessionAgents(agents, tools, "/project")).toBe(agents);
  });

  test("uses an eligible remembered agent or the first eligible agent", () => {
    const available = sessionAgents(agents, tools);
    expect(selectSessionAgent(available, "search")).toBe(agents[2]);
    expect(selectSessionAgent(available, "code")).toBe(agents[1]);
    expect(selectSessionAgent(available, "missing")).toBe(agents[1]);
    expect(selectSessionAgent(available, "")).toBe(agents[1]);
  });

  test("replaces a selection after removing its directory", () => {
    expect(selectSessionAgent(sessionAgents(agents, tools, "/project"), "code")).toBe(agents[0]);
    expect(selectSessionAgent(sessionAgents(agents, tools, ""), "code")).toBe(agents[1]);
    expect(selectSessionAgent(sessionAgents(agents, tools, "/project"), "chat")).toBe(agents[1]);
  });

  test("clears the selection when no eligible agents remain", () => {
    expect(selectSessionAgent(sessionAgents([agents[0]!], tools), "code")).toBeUndefined();
  });

  test("keeps session history accessible when the tool catalog fails", async () => {
    const session = {
      id: "session-a",
      history: [{ role: "user", content: [{ type: "text", text: "hello" }] }],
    } as Session;
    const error = new Error("Could not compose tool catalog");
    const errors: unknown[] = [];
    const [loadedSession, catalog] = await Promise.all([
      Promise.resolve(session),
      loadSessionTools(Promise.reject(error), (error) => errors.push(error)),
    ]);
    expect(loadedSession).toBe(session);
    expect(loadedSession.history).toEqual(session.history);
    expect(errors).toEqual([error]);
    expect(catalog).toBeNull();
    expect(sessionAgents(agents, catalog)).toEqual([]);
    expect(sessionAgents(agents, catalog, "/project")).toEqual([]);
    expect(selectSessionAgent(sessionAgents(agents, catalog), "code")).toBeUndefined();
  });

  test("uses the loaded tool catalog and distinguishes an empty catalog from failure", async () => {
    const onError = () => {
      throw new Error("unexpected catalog error");
    };
    expect(await loadSessionTools(Promise.resolve({ tools }), onError)).toBe(tools);
    const catalog = await loadSessionTools(Promise.resolve({ tools: [] }), onError);
    expect(catalog).toEqual([]);
    expect(sessionAgents(agents, catalog, "/project")).toBe(agents);
  });
});

test("an untitled session uses its first user message on a later turn", () => {
  const session = {
    id: "ses_untitled",
    title: "New session",
    history: [{ role: "user", content: [{ type: "text", text: "What is the current weather?" }] }],
  } as Session;
  expect(shouldAutoGenerateTitle(session)).toBe(true);
  expect(titleSourceMessage(session, "A follow-up question")).toBe("What is the current weather?");
  expect(shouldAutoGenerateTitle({ ...session, title: "Weather in New York" })).toBe(false);
});

describe("reasoningSummary", () => {
  test("extracts a provider reasoning-summary heading", () => {
    expect(
      reasoningSummary("**Inspecting client identity**\n\nChecking persisted client attribution."),
    ).toEqual({
      title: "Inspecting client identity",
      body: "Checking persisted client attribution.",
    });
  });

  test("keeps reasoning without a heading as the body", () => {
    expect(reasoningSummary("Checking persisted client attribution.")).toEqual({
      title: "",
      body: "Checking persisted client attribution.",
    });
  });
});

describe("shouldShowThinking", () => {
  test("only shows the generic status before visible activity", () => {
    expect(shouldShowThinking(true, false)).toBe(true);
    expect(shouldShowThinking(true, true)).toBe(false);
    expect(shouldShowThinking(false, false)).toBe(false);
  });
});

describe("withFailedUserMessage", () => {
  test("retains a rejected submission in an empty transcript", () => {
    expect(withFailedUserMessage([], "hello")).toEqual([
      { role: "user", content: [{ type: "text", text: "hello" }] },
    ]);
  });

  test("does not duplicate a persisted user message", () => {
    const messages = [
      { role: "user" as const, content: [{ type: "text" as const, text: "hello" }] },
    ];
    expect(withFailedUserMessage(messages, "hello")).toBe(messages);
  });
});

test("formatSessionError explains how to fix a missing working directory", () => {
  expect(
    formatSessionError(
      new Error(
        'session cannot start: tool "read" requires a working directory, but session has none',
      ),
    ),
  ).toBe(
    "This session has no working directory. Set its working directory before using this agent.",
  );
});

test("modelRefExists accepts only advertised variants", () => {
  const models = {
    openai: [{ id: "gpt-5.6-terra", variants: ["low", "high"] }],
  } as Parameters<typeof modelRefExists>[0];
  expect(modelRefExists(models, "openai/gpt-5.6-terra")).toBe(true);
  expect(modelRefExists(models, "openai/gpt-5.6-terra#high")).toBe(true);
  expect(modelRefExists(models, "openai/gpt-5.6-terra#max")).toBe(false);
});
