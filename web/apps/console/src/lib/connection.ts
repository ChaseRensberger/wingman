export type DaemonConnectionPhase = "connecting" | "live" | "retrying" | "paused" | "failed";

const initialRetryDelayMs = 250;
const maxRetryDelayMs = 5_000;

export function daemonRetryDelay(attempt: number): number {
  return Math.min(initialRetryDelayMs * 2 ** Math.max(0, attempt), maxRetryDelayMs);
}

export function daemonFailurePhase(attempt: number): DaemonConnectionPhase {
  return attempt >= 5 ? "failed" : "retrying";
}

export function daemonConnectionMessage(phase: DaemonConnectionPhase): string {
  switch (phase) {
    case "connecting":
      return "Connecting to Wingman";
    case "retrying":
      return "Connection lost. Reconnecting";
    case "paused":
      return "Connection checks paused while offline";
    case "failed":
      return "Wingman is unavailable. Retrying";
    case "live":
      return "Connected to Wingman";
  }
}
