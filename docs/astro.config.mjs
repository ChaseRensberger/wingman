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
        {
          icon: "discord",
          label: "Discord",
          href: "https://discord.gg/Mw4KURek3Q",
        },
      ],
      sidebar: [
        {
          label: "Start Here",
          items: [
            { label: "Introduction", link: "/" },
            { label: "Quick Start", slug: "start-here/quickstart" },
            { label: "What Wingman Is", slug: "start-here/what-is-wingman" },
          ],
        },
        {
          label: "Use Wingman",
          items: [
            { label: "Run the Server", slug: "use-wingman/run-server" },
            { label: "Use the Web UI", slug: "use-wingman/web-ui" },
          ],
        },
        {
          label: "Build Clients",
          items: [
            { label: "HTTP API Basics", slug: "build-clients/http-api-basics" },
            { label: "Streaming Events", slug: "build-clients/streaming-events" },
          ],
        },
        {
          label: "Configure",
          items: [
            { label: "Global Config", slug: "start-here/config" },
            { label: "Providers", slug: "configure/providers" },
            { label: "Models", slug: "configure/models" },
          ],
        },
        {
          label: "Extend",
          items: [
            { label: "Go Plugin Quickstart", slug: "reference/plugin-quickstart" },
            { label: "RPC Plugin Protocol", slug: "reference/rpc-plugin-protocol" },
            { label: "Plugin Capabilities", slug: "reference/plugin-capabilities" },
          ],
        },
        {
          label: "Concepts",
          items: [
            { label: "Clients", slug: "concepts/clients" },
            { label: "Sessions", slug: "concepts/sessions" },
            { label: "Workspaces", slug: "concepts/workspaces" },
            { label: "Agents", slug: "concepts/agents" },
            { label: "Tools", slug: "concepts/tools" },
            { label: "Plugins", slug: "concepts/plugins" },
            { label: "WingModels", slug: "concepts/wingmodels" },
            { label: "Storage", slug: "concepts/storage" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "CLI", slug: "reference/cli" },
            { label: "API", slug: "reference/referenceapi" },
            { label: "Config Schema", slug: "reference/config-schema" },
          ],
        },
      ],
    }),
  ],
});
