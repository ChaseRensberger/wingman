export type DaemonConnectionPhase = "connecting" | "live" | "retrying" | "paused" | "failed";

const initialRetryDelayMs = 250;
const maxRetryDelayMs = 5_000;

export function daemonRetryDelay(attempt: number): number {
  return Math.min(initialRetryDelayMs * 2 ** Math.max(0, attempt), maxRetryDelayMs);
}

export function daemonFailurePhase(attempt: number): DaemonConnectionPhase {
  return attempt >= 5 ? "failed" : "retrying";
}

export function daemonConnectionFailureMessage(error: unknown): string | undefined {
  const status = error && typeof error === "object" && "status" in error && typeof error.status === "number" ? error.status : undefined;
  if (status === 401) return "This Console is not paired with Wingman (401). Paste a pairing link or credential below.";
  if (status === 503) return "Wingman is starting or recovering its durable state.";
}

export function daemonConnectionMessage(phase: DaemonConnectionPhase, failure?: string): string {
  if (failure && (phase === "retrying" || phase === "failed")) return `${failure} Retrying.`;
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
