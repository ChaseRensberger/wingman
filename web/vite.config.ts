import { defineConfig } from "vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [
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
      "/health": { target: "http://127.0.0.1:2323", changeOrigin: true },
      "/provider": { target: "http://127.0.0.1:2323", changeOrigin: true },
      "/agents": { target: "http://127.0.0.1:2323", changeOrigin: true },
      "/clients": { target: "http://127.0.0.1:2323", changeOrigin: true },
      "/sessions": { target: "http://127.0.0.1:2323", changeOrigin: true },
    },
  },
});
