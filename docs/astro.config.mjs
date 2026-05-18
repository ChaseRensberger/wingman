// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// https://astro.build/config
export default defineConfig({
  site: "https://docs.wingman.actor",
  integrations: [
    starlight({
      title: "Wingman",
      // logo: {
      //   src: "./src/assets/icon-512.png",
      // },
      components: {
        PageTitle: "./src/components/PageTitle.astro",
        SiteTitle: "./src/components/SiteTitle.astro",
        ThemeProvider: "./src/components/ThemeProvider.astro",
        ThemeSelect: "./src/components/ThemeSelect.astro",
      },
      customCss: ["./src/styles/custom.css"],
      favicon: "/icon-32.png",
      pagination: false,
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/chaserensberger/wingman",
        },
        // {
        //   icon: "discord",
        //   label: "Discord",
        //   href: "",
        // },
      ],
      sidebar: [
        { label: "Introduction", link: "/" },
        { label: "Quick Start", slug: "quickstart" },
        {
          label: "Core",
          items: [
            { label: "Clients", slug: "core/clients" },
            { label: "Sessions", slug: "core/sessions" },
            { label: "Agents", slug: "core/agents" },
            { label: "WingModels", slug: "core/wingmodels" },
            { label: "Plugins", slug: "core/plugins" },
            { label: "Storage", slug: "core/storage" },
            { label: "Tools", slug: "core/tools" },
          ],
        },
        {
          label: "Editorial",
          items: [
            {
              label: "Build a Coding TUI with Wingman",
              slug: "editorial/build-coding-tui-with-wingman",
            },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "CLI", slug: "reference/cli" },
            { label: "API", slug: "reference/referenceapi" },
          ],
        },
      ],
    }),
  ],
});
