import { useEffect, useRef, useState, type ReactNode } from "react";

import { client, daemonConnectionFailureEvent } from "@/lib/client";
import {
  daemonConnectionFailureMessage,
  daemonConnectionMessage,
  daemonFailurePhase,
  daemonRetryDelay,
} from "@/lib/connection";
import {
  DaemonConnectionContext,
  useDaemonConnection,
  type DaemonConnection,
} from "@/components/daemon-connection-context";
import { queryClient } from "@/lib/query-client";

export function DaemonConnectionProvider({ children }: { children: ReactNode }) {
  const [connection, setConnection] = useState<DaemonConnection>({
    phase: "connecting",
    revision: 0,
    hasConnected: false,
  });
  const hasBeenLive = useRef(false);
  const disconnected = useRef(false);
  useEffect(() => {
    let stopped = false;
    let attempt = 0;
    let timer: number | undefined;
    let request: AbortController | undefined;

    function clearPending() {
      if (timer !== undefined) window.clearTimeout(timer);
      timer = undefined;
      request?.abort();
      request = undefined;
    }

    function schedule(delay: number) {
      if (stopped) return;
      if (timer !== undefined) window.clearTimeout(timer);
      timer = window.setTimeout(probe, delay);
    }

    async function probe() {
      if (stopped) return;
      clearPending();
      if (!navigator.onLine) {
        disconnected.current = hasBeenLive.current;
        setConnection((current) => ({ ...current, phase: "paused" }));
        return;
      }
      if (document.visibilityState === "hidden") {
        setConnection((current) => ({ ...current, phase: "paused" }));
        return;
      }

      if (!hasBeenLive.current || attempt > 0 || disconnected.current) {
        setConnection((current) => ({
          ...current,
          phase: attempt === 0 && !hasBeenLive.current ? "connecting" : "retrying",
        }));
      }
      const controller = new AbortController();
      request = controller;
      try {
        const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(3_000)]);
        const ready = await client.health.ready({ signal });
        if (signal.aborted) return;
        if (!ready.ready) throw new Error("Wingman is not ready");
        const recovered = hasBeenLive.current && disconnected.current;
        hasBeenLive.current = true;
        disconnected.current = false;
        attempt = 0;
        setConnection((current) => ({
          phase: "live",
          revision: current.revision + (recovered ? 1 : 0),
          hasConnected: true,
        }));
        if (recovered) void queryClient.invalidateQueries();
        schedule(5_000);
      } catch (error) {
        if (stopped || (error as Error).name === "AbortError") return;
        disconnected.current = hasBeenLive.current;
        attempt += 1;
        const failure = daemonConnectionFailureMessage(error);
        setConnection((current) => ({
          ...current,
          phase: daemonFailurePhase(attempt),
          failure,
        }));
        schedule(daemonRetryDelay(attempt - 1));
      } finally {
        if (request === controller) request = undefined;
      }
    }

    function resume() {
      if (!navigator.onLine) {
        clearPending();
        disconnected.current = hasBeenLive.current;
        setConnection((current) => ({ ...current, phase: "paused" }));
        return;
      }
      if (document.visibilityState === "hidden") {
        clearPending();
        setConnection((current) => ({ ...current, phase: "paused" }));
        return;
      }
      schedule(0);
    }

    function connectionFailed() {
      disconnected.current = hasBeenLive.current;
      if (!request) schedule(0);
    }

    window.addEventListener("online", resume);
    window.addEventListener("offline", resume);
    window.addEventListener(daemonConnectionFailureEvent, connectionFailed);
    document.addEventListener("visibilitychange", resume);
    schedule(0);
    return () => {
      stopped = true;
      clearPending();
      window.removeEventListener("online", resume);
      window.removeEventListener("offline", resume);
      window.removeEventListener(daemonConnectionFailureEvent, connectionFailed);
      document.removeEventListener("visibilitychange", resume);
    };
  }, []);

  return <DaemonConnectionContext value={connection}>{children}</DaemonConnectionContext>;
}

export function DaemonConnectionBanner() {
  const { phase, failure } = useDaemonConnection();
  if (phase === "live") return null;

  return (
    <div
      className="border-b border-amber-500/25 bg-amber-500/10 px-4 py-2 text-center text-xs text-amber-800 dark:text-amber-200"
      role="status"
    >
      <p>{daemonConnectionMessage(phase, failure)}</p>
    </div>
  );
}
