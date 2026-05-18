import path from "path";
import { copyFile, mkdir } from "node:fs/promises";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";

function copyInstallScript(): Plugin {
  return {
    name: "copy-install-script",
    async closeBundle() {
      const outDir = path.resolve(__dirname, "dist");

      await mkdir(outDir, { recursive: true });
      await copyFile(path.resolve(__dirname, "../install"), path.join(outDir, "install"));
    },
  };
}

export default defineConfig({
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react({
      babel: {
        plugins: [["babel-plugin-react-compiler"]],
      },
    }),
    tailwindcss(),
    copyInstallScript(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
