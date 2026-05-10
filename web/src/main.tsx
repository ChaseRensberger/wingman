import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import "./globals.css";
import { ensureClient } from "./lib/client";
import { router } from "./router";
import { ThemeProvider } from "./components/theme-provider";

async function main() {
  await ensureClient();
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <ThemeProvider>
        <RouterProvider router={router} />
      </ThemeProvider>
    </StrictMode>,
  );
}

main().catch((err) => {
  console.error("Bootstrap failed:", err);
  document.body.innerHTML = `<div style="padding:2rem">Failed to start: ${String(err)}</div>`;
});
