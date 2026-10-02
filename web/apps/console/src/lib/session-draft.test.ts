import { afterEach, expect, test } from "bun:test";
import { draftKey, matchingSubmission, readDraft, saveDraft } from "./session-draft";

const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
afterEach(() => {
  if (original) Object.defineProperty(globalThis, "localStorage", original);
  else Reflect.deleteProperty(globalThis, "localStorage");
});

test("drafts and uncertain request IDs survive reloads and remain session scoped", () => {
  const data = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => data.get(key) ?? null,
      setItem: (key: string, value: string) => data.set(key, value),
      removeItem: (key: string) => data.delete(key),
    },
  });
  const key = draftKey("ses_1");
  const pending = {
    requestId: "req_1",
    sessionId: "ses_1",
    agentId: "agent_1",
    modelRef: "test/model",
    message: "hello",
  };
  saveDraft(key, { text: "hello", pending });
  expect(readDraft(key)).toEqual({ text: "hello", pending });
  expect(readDraft(draftKey("ses_2")).text).toBe("");
  expect(draftKey("new", "ws_1")).not.toBe(draftKey("new", "ws_2"));
  expect(
    matchingSubmission(readDraft(key).pending, "ses_1", "agent_1", "test/model", "hello"),
  ).toBe("req_1");
  expect(matchingSubmission(pending, "ses_1", "agent_1", "test/model", "changed")).toBeUndefined();
  saveDraft(key, { text: "" });
  expect(readDraft(key)).toEqual({ text: "" });
});

test("unavailable storage does not break the composer", () => {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get() {
      throw new Error("storage disabled");
    },
  });
  expect(readDraft("draft")).toEqual({ text: "" });
  expect(() => saveDraft("draft", { text: "hello" })).not.toThrow();
});
