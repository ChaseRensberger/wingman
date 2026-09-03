import { useEffect, useRef, useState, type ReactNode } from "react";

import { client, daemonConnectionFailureEvent, daemonRestartRequestedEvent } from "@/lib/client";
import {
  daemonConnectionFailureMessage,
  daemonConnectionMessage,
  daemonFailurePhase,
  daemonRetryDelay,
  isReplacementInstance,
} from "@/lib/connection";
import {
  DaemonConnectionContext,
  useDaemonConnection,
  type DaemonConnection,
} from "@/components/daemon-connection-context";
import { queryClient } from "@/lib/query-client";
import { toastManager } from "@/lib/toast";

export function DaemonConnectionProvider({ children }: { children: ReactNode }) {
  const [connection, setConnection] = useState<DaemonConnection>({
    phase: "connecting",
    revision: 0,
    hasConnected: false,
  });
  const hasBeenLive = useRef(false);
  const disconnected = useRef(false);
  const instanceID = useRef("");
  const restartRequested = useRef(false);
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
        if (restartRequested.current && ready.instance_id === instanceID.current) {
          setConnection((current) => ({ ...current, phase: "restarting" }));
          schedule(daemonRetryDelay(0));
          return;
        }
        const restarted =
          restartRequested.current && isReplacementInstance(instanceID.current, ready.instance_id);
        const recovered = hasBeenLive.current && (disconnected.current || restarted);
        instanceID.current = ready.instance_id;
        restartRequested.current = false;
        hasBeenLive.current = true;
        disconnected.current = false;
        attempt = 0;
        setConnection((current) => ({
          phase: "live",
          revision: current.revision + (recovered ? 1 : 0),
          hasConnected: true,
        }));
        if (recovered) void queryClient.invalidateQueries();
        if (restarted)
          toastManager.add({
            title: "Service restarted",
            description: "Any active runs were aborted. Queued runs resumed.",
            type: "success",
          });
        schedule(5_000);
      } catch (error) {
        if (stopped || (error as Error).name === "AbortError") return;
        disconnected.current = hasBeenLive.current;
        attempt += 1;
        if (restartRequested.current) {
          setConnection((current) => ({ ...current, phase: "restarting" }));
          schedule(daemonRetryDelay(attempt - 1));
          return;
        }
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

    function restartService() {
      restartRequested.current = true;
      disconnected.current = true;
      attempt = 0;
      setConnection((current) => ({ ...current, phase: "restarting" }));
      clearPending();
      schedule(0);
    }

    window.addEventListener("online", resume);
    window.addEventListener("offline", resume);
    window.addEventListener(daemonConnectionFailureEvent, connectionFailed);
    window.addEventListener(daemonRestartRequestedEvent, restartService);
    document.addEventListener("visibilitychange", resume);
    schedule(0);
    return () => {
      stopped = true;
      clearPending();
      window.removeEventListener("online", resume);
      window.removeEventListener("offline", resume);
      window.removeEventListener(daemonConnectionFailureEvent, connectionFailed);
      window.removeEventListener(daemonRestartRequestedEvent, restartService);
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
