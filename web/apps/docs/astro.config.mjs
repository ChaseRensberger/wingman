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
          ],
        },
        {
          label: "Use Wingman",
          items: [
            { label: "Run the Server", slug: "use-wingman/run-server" },
            { label: "Use the Console", slug: "use-wingman/web-ui" },
          ],
        },
        {
          label: "Build Clients",
          items: [
            { label: "HTTP API Basics", slug: "build-clients/http-api-basics" },
			{ label: "Go SDK", slug: "build-clients/go-sdk" },
            { label: "TypeScript SDK", slug: "build-clients/typescript-sdk" },
            {
              label: "Streaming Events",
              slug: "build-clients/streaming-events",
            },
          ],
        },
        {
          label: "Configure",
          items: [
            { label: "Global Config", slug: "configure/config" },
            { label: "Providers", slug: "configure/providers" },
            { label: "Models", slug: "configure/models" },
            { label: "MCP Servers", slug: "configure/mcp" },
            { label: "Permissions", slug: "configure/permissions" },
          ],
        },
        {
          label: "Extend",
          items: [
            { label: "Go Plugin Quickstart", slug: "extend/plugin-quickstart" },
            {
              label: "RPC Plugin Protocol",
              slug: "extend/rpc-plugin-protocol",
            },
            {
              label: "Plugin Capabilities",
              slug: "extend/plugin-capabilities",
            },
            { label: "Embed the Daemon", slug: "extend/embed-wingman" },
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
            { label: "Authentication", slug: "concepts/authentication" },
            { label: "Storage", slug: "concepts/storage" },
            { label: "Execution Scopes", slug: "concepts/execution-scopes" },
            { label: "Durable Events", slug: "concepts/durable-events" },
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
