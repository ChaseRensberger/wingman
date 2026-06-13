import { createRootRoute } from "@tanstack/react-router";
import App from "@/App";
import NotFoundPage from "@/not-found";

export const Route = createRootRoute({
  component: App,
  notFoundComponent: NotFoundPage,
});
