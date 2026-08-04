import { createContext, useCallback, useContext, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";

import { api, apiData, daemonConnectionFailureEvent } from "@/lib/client";
import { daemonConnectionFailureMessage, daemonConnectionMessage, daemonFailurePhase, daemonRetryDelay, type DaemonConnectionPhase } from "@/lib/connection";
import { pairingCredentialFromFragment, pairingCredentialFromInput, redeemPairingCredential } from "@/lib/pairing";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";

type DaemonConnection = {
  phase: DaemonConnectionPhase;
  revision: number;
  hasConnected: boolean;
  failure?: string;
  requiresPairing: boolean;
  pairingError?: string;
  redeemPairing: (input: string) => Promise<void>;
};

const DaemonConnectionContext = createContext<DaemonConnection>({
  phase: "connecting",
  revision: 0,
  hasConnected: false,
  requiresPairing: false,
  redeemPairing: async () => {},
});

export function DaemonConnectionProvider({ children }: { children: ReactNode }) {
  const [connection, setConnection] = useState<DaemonConnection>({ phase: "connecting", revision: 0, hasConnected: false, requiresPairing: false, redeemPairing: async () => {} });
  const hasBeenLive = useRef(false);
  const disconnected = useRef(false);
  const retryReadiness = useRef<() => void>(() => {});

  const retry = useCallback(() => retryReadiness.current(), []);
  const redeemPairing = useCallback(async (input: string) => {
    const credential = pairingCredentialFromInput(input);
    if (!credential) {
      const error = "Paste a pairing link that includes a pairing credential, or paste the credential itself.";
      setConnection((current) => ({ ...current, pairingError: error }));
      throw new Error(error);
    }

    try {
      await redeemPairingCredential(credential);
      setConnection((current) => ({ ...current, pairingError: undefined }));
      retry();
    } catch (error) {
      const detail = error instanceof Error ? error.message : "The pairing request could not be completed.";
      setConnection((current) => ({ ...current, pairingError: `Could not pair this Console: ${detail}. Paste a new pairing link or credential and try again.` }));
      throw error;
    }
  }, [retry]);

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
        setConnection((current) => ({ ...current, phase: attempt === 0 && !hasBeenLive.current ? "connecting" : "retrying" }));
      }
      const controller = new AbortController();
      request = controller;
      try {
        const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(3_000)]);
        const ready = await apiData(api.GET("/ready", { signal }));
        if (!ready.ready) throw new Error("Wingman is not ready");
        const recovered = hasBeenLive.current && disconnected.current;
        hasBeenLive.current = true;
        disconnected.current = false;
        attempt = 0;
        setConnection((current) => ({ phase: "live", revision: current.revision + (recovered ? 1 : 0), hasConnected: true, requiresPairing: false, pairingError: undefined, redeemPairing }));
        schedule(5_000);
      } catch (error) {
        if (stopped || (error as Error).name === "AbortError") return;
        disconnected.current = hasBeenLive.current;
        attempt += 1;
        const failure = daemonConnectionFailureMessage(error);
        const requiresPairing = Boolean(error && typeof error === "object" && "status" in error && error.status === 401);
        setConnection((current) => ({ ...current, phase: daemonFailurePhase(attempt), failure, requiresPairing }));
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

    retryReadiness.current = () => {
      clearPending();
      attempt = 0;
      schedule(0);
    };
    window.addEventListener("online", resume);
    window.addEventListener("offline", resume);
    window.addEventListener(daemonConnectionFailureEvent, connectionFailed);
    document.addEventListener("visibilitychange", resume);
    schedule(0);
    return () => {
      stopped = true;
      retryReadiness.current = () => {};
      clearPending();
      window.removeEventListener("online", resume);
      window.removeEventListener("offline", resume);
      window.removeEventListener(daemonConnectionFailureEvent, connectionFailed);
      document.removeEventListener("visibilitychange", resume);
    };
  }, [redeemPairing]);

  useEffect(() => {
    const { credential, fragment } = pairingCredentialFromFragment(window.location.hash);
    if (!credential) return;

    window.history.replaceState(window.history.state, "", `${window.location.pathname}${window.location.search}${fragment}`);
    void redeemPairing(credential).catch(() => {});
  }, [redeemPairing]);

  return <DaemonConnectionContext value={{ ...connection, redeemPairing }}>{children}</DaemonConnectionContext>;
}

export function useDaemonConnection(): DaemonConnection {
  return useContext(DaemonConnectionContext);
}

export function DaemonConnectionBanner() {
  const { phase, failure, pairingError, redeemPairing, requiresPairing } = useDaemonConnection();
  const [pairingInput, setPairingInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  if (phase === "live") return null;

  async function submitPairing(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    try {
      await redeemPairing(pairingInput);
      setPairingInput("");
    } catch {
      // The provider records an actionable pairing error for the banner.
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="border-b border-amber-500/25 bg-amber-500/10 px-4 py-2 text-center text-xs text-amber-800 dark:text-amber-200" role="status">
      <p>{pairingError ?? daemonConnectionMessage(phase, failure)}</p>
      {requiresPairing && (
        <form className="mx-auto mt-2 flex max-w-lg flex-wrap items-center justify-center gap-2" onSubmit={submitPairing}>
          <Input
            className="h-7 min-w-52 flex-1 bg-background text-xs"
            value={pairingInput}
            onChange={(event) => setPairingInput(event.target.value)}
            placeholder="Paste pairing link or credential"
            autoComplete="off"
            spellCheck={false}
            aria-label="Pairing link or credential"
          />
          <Button size="sm" type="submit" disabled={submitting || !pairingInput.trim()}>{submitting ? "Pairing..." : "Pair"}</Button>
        </form>
      )}
    </div>
  );
}
