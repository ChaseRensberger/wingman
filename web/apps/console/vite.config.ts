import { defineConfig } from "vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import os from "node:os";
import path from "path";

import { readDaemonProxy } from "./daemon-proxy";

const stateDir = path.join(
  process.env.XDG_STATE_HOME ?? path.join(os.homedir(), ".local", "state"),
  "wingman",
);
const serviceConfigPath = path.join(
  process.env.XDG_CONFIG_HOME ?? path.join(os.homedir(), ".config"),
  "wingman",
  "service.env",
);
const {
  target: daemonTarget,
  username: daemonUsername,
  password: daemonPassword,
} = readDaemonProxy(stateDir, serviceConfigPath);
const daemonProxy = () => ({
  target: daemonTarget,
  changeOrigin: true,
  headers: daemonPassword
    ? {
        Authorization: `Basic ${Buffer.from(`${daemonUsername}:${daemonPassword}`).toString("base64")}`,
      }
    : undefined,
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
      "/service": daemonProxy(),
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
