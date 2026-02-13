import path from "path";
import fs from "fs";
import type { ViteDevServer } from "vite";

type FrontmatterEntry = Record<string, string | number>;

interface MarkdownPluginOptions {
  name: string;
  virtualModuleId: string;
  contentDir: string;
  fields: string[];
  defaults?: FrontmatterEntry;
  exportName: string;
  sort?: (a: FrontmatterEntry, b: FrontmatterEntry) => number;
}

function parseFrontmatterField(
  frontmatter: string,
  field: string,
): string | undefined {
  const match = frontmatter.match(
    new RegExp(`${field}:\\s*["']?(.+?)["']?\\s*$`, "m"),
  );
  return match?.[1];
}

export function markdownPlugin(options: MarkdownPluginOptions) {
  const resolvedVirtualModuleId = "\0" + options.virtualModuleId;
  return {
    name: options.name,
    resolveId(id: string) {
      if (id === options.virtualModuleId) {
        return resolvedVirtualModuleId;
      }
    },
    load(id: string) {
      if (id === resolvedVirtualModuleId) {
        const files = fs
          .readdirSync(options.contentDir)
          .filter((f) => f.endsWith(".md"));
        const items = files
          .map((file) => {
            const raw = fs.readFileSync(
              path.join(options.contentDir, file),
              "utf-8",
            );
            const slug = file.replace(/\.md$/, "");
            const frontmatterMatch = raw.match(
              /^---\n([\s\S]*?)\n---\n([\s\S]*)$/,
            );
            let draft = false;
            let body = raw;
            const entry: FrontmatterEntry = {
              slug,
              ...options.defaults,
            };

            if (frontmatterMatch) {
              const frontmatter = frontmatterMatch[1];
              body = frontmatterMatch[2];
              const draftMatch = frontmatter.match(/draft:\s*(true|false)/m);
              if (draftMatch) draft = draftMatch[1] === "true";

              for (const field of options.fields) {
                const value = parseFrontmatterField(frontmatter, field);
                if (value !== undefined) {
                  entry[field] =
                    field === "order" ? parseInt(value, 10) : value;
                }
              }
            }

            if (draft) return null;
            entry.content = body;
            return entry;
          })
          .filter((item): item is FrontmatterEntry => item !== null);

        if (options.sort) {
          items.sort(options.sort);
        }

        return `export const ${options.exportName} = ${JSON.stringify(items)};`;
      }
    },
    handleHotUpdate({ file, server }: { file: string; server: ViteDevServer }) {
      if (file.startsWith(options.contentDir) && file.endsWith(".md")) {
        const mod = server.moduleGraph.getModuleById(resolvedVirtualModuleId);
        if (mod) {
          server.moduleGraph.invalidateModule(mod);
          server.ws.send({ type: "full-reload" });
        }
      }
    },
  };
}

export function docsPlugin() {
  return markdownPlugin({
    name: "vite-plugin-docs",
    virtualModuleId: "virtual:docs",
    contentDir: path.resolve(__dirname, "../../resources/docs"),
    fields: ["title", "group", "order"],
    defaults: { title: "", group: "Uncategorized", order: 999 },
    exportName: "docs",
  });
}

export function blogPlugin() {
  return markdownPlugin({
    name: "vite-plugin-blog",
    virtualModuleId: "virtual:blog",
    contentDir: path.resolve(__dirname, "../../resources/blog"),
    fields: ["title", "date", "description"],
    exportName: "posts",
    sort: (a, b) => (b.date > a.date ? 1 : -1),
  });
}
