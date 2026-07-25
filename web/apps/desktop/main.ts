import { consoleAssets } from "./assets.generated.ts";

const daemonOrigin = "http://127.0.0.1:2323";
const consolePrefix = "/console";

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

function proxy(request: Request): Promise<Response> {
  const url = new URL(request.url);
  return fetch(new Request(`${daemonOrigin}${url.pathname}${url.search}`, request));
}

Deno.serve({ hostname: "127.0.0.1" }, async (request) => {
  const url = new URL(request.url);
  if (url.pathname === "/") return Response.redirect(new URL("/console/", url), 302);
  if (url.pathname === consolePrefix || url.pathname.startsWith(`${consolePrefix}/`)) {
    return consoleAsset(url.pathname);
  }
  try {
    return await proxy(request);
  } catch {
    return new Response("Wingman daemon is unavailable at http://127.0.0.1:2323", { status: 503 });
  }
});
