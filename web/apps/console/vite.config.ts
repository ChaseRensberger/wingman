import { defineConfig } from "vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import os from "node:os";
import path from "path";
import { VitePWA } from "vite-plugin-pwa";

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
    VitePWA({
      strategies: "injectManifest",
      srcDir: "src",
      filename: "sw.ts",
      manifestFilename: "manifest.json",
      injectRegister: false,
      registerType: "prompt",
      scope: "/console/",
      manifest: {
        id: "/console/",
        name: "Wingman",
        short_name: "Wingman",
        start_url: "/console/",
        scope: "/console/",
        display: "standalone",
        theme_color: "#18181b",
        background_color: "#18181b",
        icons: [
          { src: "icon-192.png", sizes: "192x192", type: "image/png" },
          { src: "icon-512.png", sizes: "512x512", type: "image/png" },
          {
            src: "icon-512-maskable.png",
            sizes: "512x512",
            type: "image/png",
            purpose: "maskable",
          },
        ],
      },
      useCredentials: true,
      injectManifest: {
        globPatterns: ["**/*.{js,css,html,png,svg,woff,woff2,ttf}", "manifest.json"],
        maximumFileSizeToCacheInBytes: 8 * 1024 * 1024,
      },
    }),
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
      "/actions": daemonProxy(),
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
