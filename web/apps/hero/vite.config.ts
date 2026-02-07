import path from "path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import fs from "fs";

function docsPlugin() {
  const virtualModuleId = "virtual:docs";
  const resolvedVirtualModuleId = "\0" + virtualModuleId;
  const docsDir = path.resolve(__dirname, "../../../docs");
  return {
    name: "vite-plugin-docs",
    resolveId(id: string) {
      if (id === virtualModuleId) {
        return resolvedVirtualModuleId;
      }
    },
    load(id: string) {
      if (id === resolvedVirtualModuleId) {
        const files = fs.readdirSync(docsDir).filter((f) => f.endsWith(".md"));
        const docs = files
          .map((file) => {
            const content = fs.readFileSync(path.join(docsDir, file), "utf-8");
            const slug = file.replace(/\.md$/, "");
            const frontmatterMatch = content.match(
              /^---\n([\s\S]*?)\n---\n([\s\S]*)$/,
            );
            let title = slug;
            let group = "Uncategorized";
            let order = 999;
            let draft = false;
            let body = content;
            if (frontmatterMatch) {
              const frontmatter = frontmatterMatch[1];
              body = frontmatterMatch[2];
              const titleMatch = frontmatter.match(
                /title:\s*["']?(.+?)["']?\s*$/m,
              );
              const groupMatch = frontmatter.match(
                /group:\s*["']?(.+?)["']?\s*$/m,
              );
              const orderMatch = frontmatter.match(/order:\s*(\d+)/m);
              const draftMatch = frontmatter.match(/draft:\s*(true|false)/m);
              if (titleMatch) title = titleMatch[1];
              if (groupMatch) group = groupMatch[1];
              if (orderMatch) order = parseInt(orderMatch[1], 10);
              if (draftMatch) draft = draftMatch[1] === "true";
            }
            if (draft) return null;
            return { slug, title, group, order, content: body };
          })
          .filter(Boolean);
        return `export const docs = ${JSON.stringify(docs)};`;
      }
    },
    handleHotUpdate({ file, server }: { file: string; server: any }) {
      if (file.startsWith(docsDir) && file.endsWith(".md")) {
        const mod = server.moduleGraph.getModuleById(resolvedVirtualModuleId);
        if (mod) {
          server.moduleGraph.invalidateModule(mod);
          server.ws.send({ type: "full-reload" });
        }
      }
    },
  };
}

export default defineConfig({
  plugins: [
    docsPlugin(),
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
