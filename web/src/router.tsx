import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import App from "@/App";
import AgentsPage from "@/routes/agents";
import HomePage from "@/routes/home";
import NotFoundPage from "@/routes/not-found";
import ProvidersPage from "@/routes/providers";
import SessionsPage from "@/routes/sessions";
import SessionDetailPage from "@/routes/session-detail";

const rootRoute = createRootRoute({
  component: App,
  notFoundComponent: NotFoundPage,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

const sessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sessions",
  component: SessionsPage,
});

const agentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents",
  component: AgentsPage,
});

const providersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/providers",
  component: ProvidersPage,
});

const sessionDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sessions/$sessionId",
  component: SessionDetailPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  agentsRoute,
  providersRoute,
  sessionsRoute,
  sessionDetailRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
