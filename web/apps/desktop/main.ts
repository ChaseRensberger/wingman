import { consoleAssets } from "./assets.generated.ts";

const consolePrefix = "/console";

type DaemonRegistration = {
  instance_id: string;
  version: string;
  url: string;
  pid: number;
  created_at: string;
};

type DaemonReadiness = {
  ready: boolean;
  instance_id: string;
  version: string;
};

type DaemonTransport = { origin: string; credential: string };

let cachedTransport: { value: DaemonTransport; expiresAt: number } | undefined;
let pendingTransport: Promise<DaemonTransport> | undefined;

function daemonStateDir(): string {
  const state = Deno.env.get("XDG_STATE_HOME");
  if (state) return `${state}/wingman`;
  const home = Deno.env.get("HOME") ?? Deno.env.get("USERPROFILE");
  if (!home) throw new Error("unable to resolve the user home directory");
  return `${home}/.local/state/wingman`;
}

async function daemonTransport(): Promise<DaemonTransport> {
  if (cachedTransport && cachedTransport.expiresAt > Date.now()) return cachedTransport.value;
  if (pendingTransport) return pendingTransport;
  const pending = loadDaemonTransport();
  pendingTransport = pending;
  try {
    return await pending;
  } finally {
    if (pendingTransport === pending) pendingTransport = undefined;
  }
}

async function loadDaemonTransport(): Promise<DaemonTransport> {
  const state = daemonStateDir();
  const [rawRegistration, rawCredential] = await Promise.all([
    Deno.readTextFile(`${state}/registration.json`),
    Deno.readTextFile(`${state}/credential`),
  ]);
  const registration = JSON.parse(rawRegistration) as DaemonRegistration;
  const origin = new URL(registration.url);
  if ((origin.protocol !== "http:" && origin.protocol !== "https:") || !registration.instance_id) {
    throw new Error("invalid Wingman daemon registration");
  }
  const credential = rawCredential.trim();
  if (!credential) throw new Error("invalid Wingman daemon credential");

  const response = await fetch(`${origin.origin}/ready`, {
    headers: { Authorization: `Bearer ${credential}` },
    signal: AbortSignal.timeout(3_000),
  });
  if (response.status !== 200) throw new Error(`Wingman daemon is not ready: HTTP ${response.status}`);
  const readiness = await response.json() as DaemonReadiness;
  if (!readiness.ready || readiness.instance_id !== registration.instance_id || readiness.version !== registration.version) {
    throw new Error("Wingman daemon readiness does not match registration");
  }

  const value = { origin: origin.origin, credential };
  cachedTransport = { value, expiresAt: Date.now() + 1_000 };
  return value;
}

function contentType(path: string): string {
  if (path.endsWith(".css")) return "text/css; charset=utf-8";
  if (path.endsWith(".html")) return "text/html; charset=utf-8";
  if (path.endsWith(".js")) return "text/javascript; charset=utf-8";
  if (path.endsWith(".json")) return "application/json; charset=utf-8";
  if (path.endsWith(".png")) return "image/png";
  if (path.endsWith(".svg")) return "image/svg+xml";
  if (path.endsWith(".ttf")) return "font/ttf";
  if (path.endsWith(".woff")) return "font/woff";
  if (path.endsWith(".woff2")) return "font/woff2";
  return "application/octet-stream";
}

function isLoopbackHostname(hostname: string): boolean {
  return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "[::1]";
}

function isTrustedRequest(request: Request): boolean {
  if (!isLoopbackHostname(new URL(request.url).hostname)) return false;
  const origin = request.headers.get("origin");
  if (!origin) return true;
  try {
    return isLoopbackHostname(new URL(origin).hostname);
  } catch {
    return false;
  }
}

function consoleAsset(pathname: string): Response {
  let path = pathname.slice(consolePrefix.length).replace(/^\//, "");
  if (!path || path.endsWith("/")) path += "index.html";
  if (path.includes("..")) return new Response("Not found", { status: 404 });

  let asset = consoleAssets[path];
  if (!asset && !path.includes(".")) {
    path = "index.html";
    asset = consoleAssets[path];
  }
  if (!asset) return new Response("Not found", { status: 404 });

  const headers = new Headers({ "content-type": contentType(path) });
  if (path === "index.html") {
    headers.set("cache-control", "no-store");
    return new Response(Uint8Array.fromBase64(asset), { headers });
  }
  headers.set("cache-control", "public, max-age=31536000, immutable");
  return new Response(Uint8Array.fromBase64(asset), { headers });
}

async function proxy(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const daemon = await daemonTransport();
  const target = new Request(`${daemon.origin}${url.pathname}${url.search}`, request);
  target.headers.set("Authorization", `Bearer ${daemon.credential}`);
  try {
    const response = await fetch(target);
    if (response.status === 401 || response.status === 503) cachedTransport = undefined;
    return response;
  } catch (error) {
    cachedTransport = undefined;
    throw error;
  }
}

Deno.serve({ hostname: "127.0.0.1" }, async (request) => {
  if (!isTrustedRequest(request)) return new Response("Forbidden", { status: 403 });
  const url = new URL(request.url);
  if (url.pathname === "/") return Response.redirect(new URL("/console/", url), 302);
  if (url.pathname === consolePrefix || url.pathname.startsWith(`${consolePrefix}/`)) {
    return consoleAsset(url.pathname);
  }
  try {
    return await proxy(request);
  } catch {
    return new Response("Wingman daemon is unavailable or not registered", { status: 503 });
  }
});
