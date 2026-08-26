import fs from "node:fs";

const defaultDaemonTarget = "http://127.0.0.1:2424";

export type DaemonProxy = {
  target: string;
  username: string;
  password: string;
};

export function readDaemonProxy(stateDir: string, serviceConfigPath: string): DaemonProxy {
  let target = defaultDaemonTarget;
  let username = "wingman";
  let password = "";

  try {
    const registration = JSON.parse(fs.readFileSync(`${stateDir}/registration.json`, "utf8")) as {
      url?: unknown;
    };
    if (typeof registration.url === "string" && registration.url) target = registration.url;
  } catch {
    // Foreground `wingman serve` does not register itself.
  }

  try {
    const values = fs.readFileSync(serviceConfigPath, "utf8");
    for (const line of values.trim().split("\n")) {
      const [key, value] = line.split("=", 2);
      if (key === "WINGMAN_USERNAME") username = value.slice(1, -1);
      if (key === "WINGMAN_PASSWORD") password = value.slice(1, -1);
    }
  } catch {
    // Vite may start before Wingman creates service credentials.
  }

  return { target, username, password };
}
