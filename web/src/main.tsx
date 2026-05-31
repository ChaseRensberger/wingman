import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import "./globals.css";
import { ensureClient } from "./lib/client";
import { router } from "./router";
import { ThemeProvider } from "./components/theme-provider";
import { ToastProvider } from "./components/core/toast";
import { AppToaster } from "./components/app-toaster";
import { toastManager } from "./lib/toast";

async function main() {
  await ensureClient();
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <ThemeProvider>
        <ToastProvider toastManager={toastManager}>
          <RouterProvider router={router} />
          <AppToaster />
        </ToastProvider>
      </ThemeProvider>
    </StrictMode>,
  );
}

main().catch((err) => {
  console.error("Bootstrap failed:", err);
  const message = document.createElement("div");
  message.style.padding = "2rem";
  message.textContent = `Failed to start: ${String(err)}`;
  document.body.replaceChildren(message);
});
