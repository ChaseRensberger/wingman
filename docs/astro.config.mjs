// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { rehypeHeadingIds } from "@astrojs/markdown-remark";
import theme from "toolbeam-docs-theme";

// https://astro.build/config
export default defineConfig({
  base: "/docs",
  markdown: {
    rehypePlugins: [rehypeHeadingIds],
  },
  integrations: [
    starlight({
      title: "Wingman",
      favicon: "/icon-32.png",
      expressiveCode: { themes: ["github-light", "github-dark"] },
      markdown: {
        headingLinks: false,
      },
      customCss: [
        "@fontsource/roboto-mono/400.css",
        "@fontsource/roboto-mono/400-italic.css",
        "@fontsource/roboto-mono/500.css",
        "@fontsource/roboto-mono/600.css",
        "@fontsource/roboto-mono/700.css",
        "./src/styles/custom.css",
      ],
      components: {
        Footer: "./src/components/Footer.astro",
        SiteTitle: "./src/components/SiteTitle.astro",
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/chaserensberger/wingman",
        },
        {
          icon: "discord",
          label: "Discord",
          href: "https://discord.gg/Mw4KURek3Q",
        },
      ],
      sidebar: [
        { label: "Introduction", slug: "" },
        { label: "Quickstart", slug: "quickstart" },
        {
          label: "Core",
          items: [
            { label: "Sessions", slug: "core/sessions" },
          ],
        },
      ],
      plugins: [
        theme({
          headerLinks: [],
        }),
      ],
    }),
  ],
});
