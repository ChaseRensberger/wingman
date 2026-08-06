import { defineConfig } from "vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import os from "node:os";
import path from "path";

import { readDaemonProxy } from "./daemon-proxy";

const stateDir = path.join(process.env.XDG_STATE_HOME ?? path.join(os.homedir(), ".local", "state"), "wingman");
const { target: daemonTarget, credential: daemonCredential } = readDaemonProxy(stateDir);
const daemonProxy = () => ({
  target: daemonTarget,
  changeOrigin: true,
  headers: daemonCredential ? { Authorization: `Bearer ${daemonCredential}` } : undefined,
});

export default defineConfig({
  base: "/console/",
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
    babel({ presets: [reactCompilerPreset()] }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/health": daemonProxy(),
      "/ready": daemonProxy(),
      "/auth": daemonProxy(),
      "/provider": daemonProxy(),
      "/agents": daemonProxy(),
      "/client": daemonProxy(),
      "/clients": daemonProxy(),
      "/logs": daemonProxy(),
      "/mcp": daemonProxy(),
      "/workspaces": daemonProxy(),
      "/filesystem": daemonProxy(),
      "/sessions": daemonProxy(),
      "/tools": daemonProxy(),
      "/plugins": daemonProxy(),
      "/run": daemonProxy(),
    },
  },
});
