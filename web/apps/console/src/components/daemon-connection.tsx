import { createContext, useCallback, useContext, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";

import { api, apiData, daemonConnectionFailureEvent } from "@/lib/client";
import { daemonConnectionFailureMessage, daemonConnectionMessage, daemonFailurePhase, daemonRetryDelay, type DaemonConnectionPhase } from "@/lib/connection";
import { enrollmentCredentialFromFragment, enrollmentCredentialFromInput, redeemEnrollmentCredential } from "@/lib/enrollment";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";

type DaemonConnection = {
  phase: DaemonConnectionPhase;
  revision: number;
  hasConnected: boolean;
  failure?: string;
	requiresEnrollment: boolean;
	enrollmentError?: string;
	redeemEnrollment: (input: string) => Promise<void>;
};

const DaemonConnectionContext = createContext<DaemonConnection>({
  phase: "connecting",
  revision: 0,
  hasConnected: false,
	requiresEnrollment: false,
	redeemEnrollment: async () => {},
});

export function DaemonConnectionProvider({ children }: { children: ReactNode }) {
	const [connection, setConnection] = useState<DaemonConnection>({ phase: "connecting", revision: 0, hasConnected: false, requiresEnrollment: false, redeemEnrollment: async () => {} });
  const hasBeenLive = useRef(false);
  const disconnected = useRef(false);
  const retryReadiness = useRef<() => void>(() => {});

  const retry = useCallback(() => retryReadiness.current(), []);
	const redeemEnrollment = useCallback(async (input: string) => {
		const credential = enrollmentCredentialFromInput(input);
		if (!credential) {
			const error = "Paste an enrollment credential.";
			setConnection((current) => ({ ...current, enrollmentError: error }));
      throw new Error(error);
    }

    try {
			await redeemEnrollmentCredential(credential);
			setConnection((current) => ({ ...current, enrollmentError: undefined }));
      retry();
    } catch (error) {
			const detail = error instanceof Error ? error.message : "The enrollment request could not be completed.";
			setConnection((current) => ({ ...current, enrollmentError: `Could not enroll this Console: ${detail}. Paste a new enrollment credential and try again.` }));
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
		setConnection((current) => ({ phase: "live", revision: current.revision + (recovered ? 1 : 0), hasConnected: true, requiresEnrollment: false, enrollmentError: undefined, redeemEnrollment }));
        schedule(5_000);
      } catch (error) {
        if (stopped || (error as Error).name === "AbortError") return;
        disconnected.current = hasBeenLive.current;
        attempt += 1;
        const failure = daemonConnectionFailureMessage(error);
		const requiresEnrollment = Boolean(error && typeof error === "object" && "status" in error && error.status === 401);
		setConnection((current) => ({ ...current, phase: daemonFailurePhase(attempt), failure, requiresEnrollment }));
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
	}, [redeemEnrollment]);

  useEffect(() => {
		const { credential, fragment } = enrollmentCredentialFromFragment(window.location.hash);
    if (!credential) return;

    window.history.replaceState(window.history.state, "", `${window.location.pathname}${window.location.search}${fragment}`);
		void redeemEnrollment(credential).catch(() => {});
	}, [redeemEnrollment]);

	return <DaemonConnectionContext value={{ ...connection, redeemEnrollment }}>{children}</DaemonConnectionContext>;
}

export function useDaemonConnection(): DaemonConnection {
  return useContext(DaemonConnectionContext);
}

export function DaemonConnectionBanner() {
	const { phase, failure, enrollmentError, redeemEnrollment, requiresEnrollment } = useDaemonConnection();
	const [enrollmentInput, setEnrollmentInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  if (phase === "live") return null;

	async function submitEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    try {
		await redeemEnrollment(enrollmentInput);
		setEnrollmentInput("");
    } catch {
		// The provider records an actionable enrollment error for the banner.
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="border-b border-amber-500/25 bg-amber-500/10 px-4 py-2 text-center text-xs text-amber-800 dark:text-amber-200" role="status">
		<p>{enrollmentError ?? daemonConnectionMessage(phase, failure)}</p>
		{requiresEnrollment && (
			<form className="mx-auto mt-2 flex max-w-lg flex-wrap items-center justify-center gap-2" onSubmit={submitEnrollment}>
          <Input
            className="h-7 min-w-52 flex-1 bg-background text-xs"
				value={enrollmentInput}
				onChange={(event) => setEnrollmentInput(event.target.value)}
			placeholder="Paste enrollment credential"
            autoComplete="off"
            spellCheck={false}
			aria-label="Enrollment credential"
          />
			<Button size="sm" type="submit" disabled={submitting || !enrollmentInput.trim()}>{submitting ? "Enrolling..." : "Enroll"}</Button>
        </form>
      )}
    </div>
  );
}
