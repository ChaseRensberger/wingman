/// <reference lib="webworker" />
import { addPlugins, cleanupOutdatedCaches, matchPrecache, precache } from "workbox-precaching";
import { registerRoute } from "workbox-routing";

declare const self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<{ url: string; revision: string | null }>;
};

addPlugins([
  {
    cacheWillUpdate: async ({ request, response }) => {
      // The server's SPA fallback can return HTML for a missing build asset.
      if (
        new URL(request.url).pathname !== "/console/index.html" &&
        response.headers.get("Content-Type")?.includes("text/html")
      )
        return null;
      return response.status === 200 ? response : null;
    },
  },
]);
precache(self.__WB_MANIFEST);
cleanupOutdatedCaches();

registerRoute(
  ({ url, request }) =>
    url.origin === self.location.origin &&
    url.pathname.startsWith("/console/") &&
    request.mode === "navigate",
  async ({ request }) => {
    // Authentication challenges must reach the browser, even with an installed cache.
    try {
      return await fetch(request);
    } catch {
      return (await matchPrecache("/console/index.html")) ?? Response.error();
    }
  },
);

registerRoute(
  ({ url, request }) =>
    url.origin === self.location.origin &&
    url.pathname.startsWith("/console/") &&
    request.mode !== "navigate",
  async ({ request }) => (await matchPrecache(request.url)) ?? fetch(request),
);
