import fs from "node:fs";

const defaultDaemonTarget = "http://127.0.0.1:2323";

export type DaemonProxy = {
  target: string;
	password: string;
};

export function readDaemonProxy(stateDir: string): DaemonProxy {
  let target = defaultDaemonTarget;
	let password = "";

  try {
    const registration = JSON.parse(fs.readFileSync(`${stateDir}/registration.json`, "utf8")) as { url?: unknown };
    if (typeof registration.url === "string" && registration.url) target = registration.url;
  } catch {
    // Foreground `wingman serve` does not register itself.
  }

  try {
		password = fs.readFileSync(`${stateDir}/password`, "utf8").trim();
  } catch {
		// Vite may start before Wingman creates its local password.
  }

	return { target, password };
}
