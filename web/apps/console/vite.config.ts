import { defineConfig } from "vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import fs from "node:fs";
import os from "node:os";
import path from "path";

const stateDir = path.join(process.env.XDG_STATE_HOME ?? path.join(os.homedir(), ".local", "state"), "wingman");
let daemonTarget = "http://127.0.0.1:2323";
let daemonCredential = "";
try {
  daemonTarget = JSON.parse(fs.readFileSync(path.join(stateDir, "registration.json"), "utf8")).url ?? daemonTarget;
  daemonCredential = fs.readFileSync(path.join(stateDir, "credential"), "utf8").trim();
} catch {
  // The daemon can start after Vite; restart Vite to refresh discovery state.
}
const daemonProxy = () => ({
  target: daemonTarget,
  changeOrigin: true,
  headers: daemonCredential ? { Authorization: `Bearer ${daemonCredential}` } : undefined,
});

export default defineConfig({
  base: "/console/",
  plugins: [
    tanstackRouter({ target: "react" }),
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
      "/provider": daemonProxy(),
      "/agents": daemonProxy(),
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
