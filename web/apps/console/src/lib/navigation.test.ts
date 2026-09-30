import { expect, test } from "bun:test";

import { sessionsWorkspace } from "./navigation";

test("keeps the selected workspace when switching from Sessions to Agents and back", () => {
  const selected = sessionsWorkspace("/sessions", "workspace-a");
  const away = sessionsWorkspace("/agents", undefined, selected);
  expect(away).toBe("workspace-a");
  expect(sessionsWorkspace("/sessions", away, away)).toBe("workspace-a");
});

test("remembers No workspace but clears the previous choice for All workspaces", () => {
  const selected = sessionsWorkspace("/sessions/", "none", "workspace-a");
  expect(sessionsWorkspace("/agents", undefined, selected)).toBe("none");
  const all = sessionsWorkspace("/sessions", undefined, selected);
  expect(all).toBeUndefined();
  expect(sessionsWorkspace("/agents", undefined, all)).toBeUndefined();
});

test("updates the filter from the Sessions URL but not a draft session workspace", () => {
  expect(sessionsWorkspace("/sessions", "workspace-b", "workspace-a")).toBe("workspace-b");
  expect(sessionsWorkspace("/sessions/new", "workspace-b", "workspace-a")).toBe("workspace-a");
  expect(sessionsWorkspace("/sessions", 42, "workspace-a")).toBeUndefined();
});
