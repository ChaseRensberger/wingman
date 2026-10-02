import { restorableRoute } from "./pwa-policy";

const routeKey = "wingman-pwa-route";
let registration: Promise<ServiceWorkerRegistration> | undefined;

export function isStandalone() {
  return (
    matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator && navigator.standalone === true)
  );
}

export function registerPwa() {
  if (!import.meta.env.PROD || !window.isSecureContext || !("serviceWorker" in navigator)) return;
  registration ??= navigator.serviceWorker.register("/console/sw.js", {
    scope: "/console/",
    updateViaCache: "none",
  });
  return registration;
}

export function restorePwaRoute() {
  if (!isStandalone() || location.pathname !== "/console/" || location.search || location.hash)
    return;
  try {
    const route = restorableRoute(localStorage.getItem(routeKey) ?? "", location.origin);
    if (route) history.replaceState(history.state, "", route);
  } catch {
    /* Launch still works when browser storage is unavailable. */
  }
}

export function rememberPwaRoute() {
  if (!isStandalone()) return;
  const route = restorableRoute(location.href, location.origin);
  if (!route) return;
  try {
    localStorage.setItem(routeKey, route);
  } catch {
    /* Navigation does not require storage. */
  }
}
