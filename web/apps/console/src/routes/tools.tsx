import { useEffect, useMemo, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { Badge } from "@wingman/core/components/core/badge";
import { Button } from "@wingman/core/components/core/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table";
import { Empty, EmptyDescription, EmptyTitle } from "@wingman/core/components/core/empty";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { api, apiData } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import type { MCPResponse, MCPServer, PluginsResponse, PluginStatus, ToolCatalogItem, ToolsResponse } from "@/lib/types";

export const Route = createFileRoute("/tools")({
  component: ToolsPage,
});

function ToolsPage() {
  const [tools, setTools] = useState<ToolCatalogItem[]>([]);
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [plugins, setPlugins] = useState<PluginStatus[]>([]);
  const [pluginErrors, setPluginErrors] = useState<PluginsResponse["errors"]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [reloadingPlugins, setReloadingPlugins] = useState(false);

  async function load() {
    try {
      const [toolData, mcpData, pluginData] = await Promise.all([
        apiData(api.GET("/tools")) as Promise<ToolsResponse>,
        apiData(api.GET("/mcp")) as Promise<MCPResponse>,
        apiData(api.GET("/plugins")) as Promise<PluginsResponse>,
      ]);
      setTools(toolData.tools ?? []);
      setServers(mcpData.servers ?? []);
      setPlugins(pluginData.plugins ?? []);
      setPluginErrors(pluginData.errors ?? []);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load().catch((err) => showErrorToast(err));
  }, []);

  async function mcpAction(server: MCPServer, action: "connect" | "disconnect") {
    setBusy(`${server.name}:${action}`);
    try {
      await apiData(
        action === "connect"
          ? api.POST("/mcp/{name}/connect", { params: { path: { name: server.name } } })
          : api.POST("/mcp/{name}/disconnect", { params: { path: { name: server.name } } }),
      );
      await load();
    } catch (err) {
      showErrorToast(err);
    } finally {
      setBusy(null);
    }
  }

  async function reloadPlugins() {
    setReloadingPlugins(true);
    try {
      const data = (await apiData(api.POST("/plugins/reload"))) as PluginsResponse;
      setPlugins(data.plugins ?? []);
      setPluginErrors(data.errors ?? []);
      await load();
    } catch (err) {
      showErrorToast(err);
    } finally {
      setReloadingPlugins(false);
    }
  }

  const grouped = useMemo(() => {
    const groups = new Map<string, ToolCatalogItem[]>();
    for (const tool of tools) {
      const key = tool.source === "mcp" && tool.server ? `mcp:${tool.server}` : tool.source;
      groups.set(key, [...(groups.get(key) ?? []), tool]);
    }
    return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [tools]);

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Tools" }]} />
        <p className="mt-3 max-w-2xl text-sm text-muted-foreground">
          Tools available to agents, grouped by native Wingman tools, external plugins, and MCP servers.
        </p>
      </div>

      <section className="mb-6 rounded-xl border bg-card p-4">
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">RPC plugins</h2>
            <p className="text-xs text-muted-foreground">Supervised external plugin processes and negotiated capabilities.</p>
          </div>
          <Button size="sm" variant="outline" disabled={reloadingPlugins} onClick={reloadPlugins}>
            {reloadingPlugins ? "Reloading..." : "Reload"}
          </Button>
        </div>
        {pluginErrors?.map((item) => (
          <div key={`${item.path}:${item.error}`} className="mb-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
            <span className="font-mono">{item.path}</span>: {item.error}
          </div>
        ))}
        {plugins.length === 0 ? (
          <p className="py-3 text-sm text-muted-foreground">No RPC plugins loaded.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Plugin</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Process</TableHead>
                <TableHead>Capabilities</TableHead>
                <TableHead>Tools</TableHead>
                <TableHead>Health / diagnostics</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {plugins.map((plugin) => (
                <TableRow key={plugin.id}>
                  <TableCell>
                    <div className="font-medium">{plugin.name || plugin.id}</div>
                    <div className="font-mono text-xs text-muted-foreground">{plugin.id}</div>
                  </TableCell>
                  <TableCell><Badge variant={pluginStatusVariant(plugin.status)}>{plugin.status}</Badge></TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    <div>{plugin.pid ? `PID ${plugin.pid}` : "No process"}</div>
                    <div>{plugin.plugin_version ? `v${plugin.plugin_version}` : "Unversioned"}, protocol {plugin.protocol_version ?? "-"}</div>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{plugin.capabilities?.join(", ") || "None"}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{plugin.tools?.join(", ") || "None"}</TableCell>
                  <TableCell className="max-w-sm text-xs text-muted-foreground">{pluginDiagnostic(plugin)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </section>

      {servers.length > 0 && (
        <section className="mb-6 rounded-xl border bg-card p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold">MCP servers</h2>
              <p className="text-xs text-muted-foreground">Configured in wingman.json.</p>
            </div>
            <Button size="sm" variant="outline" onClick={() => load().catch((err) => showErrorToast(err))}>
              Refresh
            </Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Tools</TableHead>
                <TableHead>Error</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {servers.map((server) => (
                <TableRow key={server.name}>
                  <TableCell className="font-medium">{server.name}</TableCell>
                  <TableCell>{server.type}</TableCell>
                  <TableCell>
                    <Badge variant={server.status === "connected" ? "default" : "outline"}>{server.status}</Badge>
                  </TableCell>
                  <TableCell>{server.tool_count}</TableCell>
                  <TableCell className="max-w-sm truncate text-xs text-muted-foreground">{server.error || "-"}</TableCell>
                  <TableCell className="text-right">
                    {server.status === "connected" ? (
                      <Button size="xs" variant="outline" disabled={busy !== null} onClick={() => mcpAction(server, "disconnect")}>
                        {busy === `${server.name}:disconnect` ? "Disconnecting..." : "Disconnect"}
                      </Button>
                    ) : (
                      <Button size="xs" variant="outline" disabled={busy !== null} onClick={() => mcpAction(server, "connect")}>
                        {busy === `${server.name}:connect` ? "Connecting..." : "Connect"}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </section>
      )}

      {loading ? (
        <div className="flex items-center gap-3 py-8 text-sm text-muted-foreground">
          <HexWaveSpinner size={24} />
          <span>Loading...</span>
        </div>
      ) : tools.length === 0 ? (
        <Empty>
          <EmptyTitle>No tools found</EmptyTitle>
          <EmptyDescription>Native tools should appear here. Check server logs if the catalog is empty.</EmptyDescription>
        </Empty>
      ) : (
        <div className="grid gap-5">
          {grouped.map(([group, items]) => (
            <section key={group} className="rounded-xl border bg-card p-4">
              <div className="mb-3 flex items-center justify-between gap-3">
                <h2 className="text-sm font-semibold">{groupLabel(group)}</h2>
                <Badge variant="outline">{items.length} tools</Badge>
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead>Input</TableHead>
                    <TableHead>Output</TableHead>
                    <TableHead>Traits</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((tool) => (
                    <TableRow key={tool.name}>
                      <TableCell className="font-mono text-xs">{tool.name}</TableCell>
                      <TableCell className="max-w-xl text-sm text-muted-foreground">{tool.description || "-"}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{tool.source}</Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{schemaSummary(tool.input_schema)}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{schemaSummary(tool.output_schema)}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{traitSummary(tool)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

function groupLabel(group: string) {
  if (group.startsWith("mcp:")) return `MCP: ${group.slice(4)}`;
  if (group === "native") return "Native";
  if (group === "plugin") return "Plugins";
  return group;
}

function schemaSummary(schema?: Record<string, unknown>) {
	if (!schema) return "-";
  const props = schema?.properties;
  if (!props || typeof props !== "object" || Array.isArray(props)) return String(schema.type ?? "schema");
  const names = Object.keys(props);
  if (names.length === 0) return "0 fields";
  return names.slice(0, 4).join(", ") + (names.length > 4 ? ` +${names.length - 4}` : "");
}

function traitSummary(tool: ToolCatalogItem) {
  const traits = [
    tool.directory_scoped ? "directory" : null,
    tool.sequential ? "sequential" : "parallel",
    tool.permission?.action ? `permission:${tool.permission.action}` : null,
  ].filter(Boolean);
  return traits.join(", ");
}

function pluginStatusVariant(status: PluginStatus["status"]) {
  if (status === "running") return "default" as const;
  if (status === "failed") return "destructive" as const;
  return "outline" as const;
}

function pluginDiagnostic(plugin: PluginStatus) {
  if (plugin.error) return plugin.error;
  const latest = plugin.diagnostics?.at(-1);
  if (latest) return `${latest.source}: ${latest.message}`;
  return plugin.health_message || "-";
}
