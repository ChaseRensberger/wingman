import { createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";
import { restorePwaRoute } from "./lib/pwa";

restorePwaRoute();

export const router = createRouter({ routeTree, basepath: "/console" });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
