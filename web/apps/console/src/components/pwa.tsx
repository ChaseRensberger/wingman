import { useEffect, useState } from "react";
import { useRouterState } from "@tanstack/react-router";
import { isStandalone, registerPwa, rememberPwaRoute } from "@/lib/pwa";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";

export function PwaLifecycle() {
  const { location } = useRouterState();
  const [update, setUpdate] = useState(false);
  useEffect(rememberPwaRoute, [location.href]);
  useEffect(() => {
    let stopped = false;
    let worker: ServiceWorkerRegistration | undefined;
    const inspect = () => {
      if (!stopped) setUpdate(!!worker?.waiting);
    };
    const installing = () => worker?.installing?.addEventListener("statechange", inspect);
    void registerPwa()
      ?.then((value) => {
        if (stopped) return;
        worker = value;
        inspect();
        installing();
        worker.addEventListener("updatefound", installing);
      })
      .catch(() => {});
    const resume = () => {
      if (document.visibilityState === "visible") void worker?.update().catch(() => {});
    };
    document.addEventListener("visibilitychange", resume);
    return () => {
      stopped = true;
      worker?.removeEventListener("updatefound", installing);
      document.removeEventListener("visibilitychange", resume);
    };
  }, []);
  useEffect(() => {
    const root = document.documentElement;
    const viewport = window.visualViewport;
    const sync = () => {
      const editing = document.activeElement?.matches("input, textarea, [contenteditable=true]");
      const keyboard =
        viewport && editing && viewport.scale === 1 && root.clientHeight - viewport.height > 100;
      root.style.setProperty("--console-height", keyboard ? `${viewport.height}px` : "100dvh");
      root.style.setProperty(
        "--console-bottom",
        keyboard ? "0px" : "env(safe-area-inset-bottom, 0px)",
      );
    };
    sync();
    viewport?.addEventListener("resize", sync);
    document.addEventListener("focusin", sync);
    document.addEventListener("focusout", sync);
    return () => {
      viewport?.removeEventListener("resize", sync);
      document.removeEventListener("focusin", sync);
      document.removeEventListener("focusout", sync);
    };
  }, []);
  return update ? (
    <div role="status" className="shrink-0 border-b bg-muted px-4 py-2 text-center text-xs">
      Update downloaded. Close all Wingman windows and reopen to use it.
    </div>
  ) : null;
}

export function PwaSettings() {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Wingman app</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {!isStandalone() && (
          <p>
            On iPhone or iPad, open the browser’s Share menu, choose Add to Home Screen, and leave
            Open as Web App enabled if shown.
          </p>
        )}
        {!window.isSecureContext && (
          <p>
            Offline launch needs HTTPS when you connect from another device. With Tailscale, use the
            HTTPS address provided by Tailscale Serve.
          </p>
        )}
        <p>
          Wingman saves unfinished messages on this device. The Home Screen app restores your last
          session when you open it. Clearing browser data removes saved drafts and offline files.
        </p>
        <p>
          The interface can open offline after its first complete download. Session history and
          agent runs still need a connection to your Wingman server.
        </p>
        {!import.meta.env.PROD && <p>Offline launch is available in the built Console.</p>}
      </CardContent>
    </Card>
  );
}
