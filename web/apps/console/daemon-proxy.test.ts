import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { readDaemonProxy } from "./daemon-proxy";

const stateDirs: string[] = [];

function stateDir() {
  const dir = mkdtempSync(join(tmpdir(), "wingman-console-"));
  stateDirs.push(dir);
  return dir;
}

afterEach(() => {
  for (const dir of stateDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

test("uses the foreground daemon default while loading service credentials", () => {
  const dir = stateDir();
  const config = join(dir, "service.env");
  writeFileSync(config, "WINGMAN_USERNAME='chase'\nWINGMAN_PASSWORD='foreground-password'\n");

  expect(readDaemonProxy(dir, config)).toEqual({
    target: "http://127.0.0.1:2424",
    username: "chase",
    password: "foreground-password",
  });
});

test("uses a managed daemon registration when available", () => {
  const dir = stateDir();
  writeFileSync(join(dir, "registration.json"), JSON.stringify({ url: "http://127.0.0.1:4545" }));
  const config = join(dir, "service.env");
  writeFileSync(config, "WINGMAN_USERNAME='wingman'\nWINGMAN_PASSWORD='managed-password'\n");

  expect(readDaemonProxy(dir, config)).toEqual({
    target: "http://127.0.0.1:4545",
    username: "wingman",
    password: "managed-password",
  });
});
