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

test("uses the foreground daemon default while loading its password", () => {
  const dir = stateDir();
	writeFileSync(join(dir, "password"), "foreground-password\n");

	expect(readDaemonProxy(dir)).toEqual({ target: "http://127.0.0.1:2323", password: "foreground-password" });
});

test("uses a managed daemon registration when available", () => {
  const dir = stateDir();
  writeFileSync(join(dir, "registration.json"), JSON.stringify({ url: "http://127.0.0.1:4545" }));
	writeFileSync(join(dir, "password"), "managed-password");

	expect(readDaemonProxy(dir)).toEqual({ target: "http://127.0.0.1:4545", password: "managed-password" });
});
