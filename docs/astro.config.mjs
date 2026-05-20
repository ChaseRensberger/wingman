import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://docs.wingman.actor",
  integrations: [
    starlight({
      title: "Wingman",
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
        {
          label: "Start Here",
          items: [
            { label: "Introduction", link: "/" },
            { label: "Quick Start", slug: "start-here/quickstart" },
            { label: "Config", slug: "start-here/config" },
          ],
        },
        // {
        //   label: "Customization",
        //   items: [
        //     // { label: "Add Plugins", slug: "customization/add-plugins" },
        //     // { label: "Add Tools", slug: "customization/add-tools" },
        //   ],
        // },
        {
          label: "Concepts",
          items: [
            { label: "Clients", slug: "concepts/clients" },
            { label: "Sessions", slug: "concepts/sessions" },
            { label: "Agents", slug: "concepts/agents" },
            { label: "WingModels", slug: "concepts/wingmodels" },
            { label: "Storage", slug: "concepts/storage" },
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
