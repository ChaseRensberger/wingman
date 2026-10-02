import { expect, test } from "bun:test";
import { restorableRoute } from "./pwa-policy";

const origin = "https://wingman.example";

test("restored routes retain workspace selection but exclude credentials and unrelated URLs", () => {
  expect(restorableRoute("/console/sessions/new?workspace=ws_1&token=secret", origin)).toBe(
    "/console/sessions/new?workspace=ws_1",
  );
  for (const value of [
    "https://other.example/console/sessions",
    "/console/settings",
    "/auth/callback",
    "https://user:secret@wingman.example/console/",
  ]) {
    expect(restorableRoute(value, origin)).toBeUndefined();
  }
});
