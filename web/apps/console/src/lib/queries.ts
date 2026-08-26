import { queryOptions } from "@tanstack/react-query";

import { client } from "@/lib/client";
import type {
  Agent,
  Client,
  MCPResponse,
  PluginsResponse,
  Provider,
  ProviderModel,
  SessionSummary,
  ToolsResponse,
  Workspace,
} from "@/lib/types";

export const queryKeys = {
  agents: ["agents"] as const,
  agent: (id: string) => ["agents", id] as const,
  clients: ["clients"] as const,
  providers: ["providers"] as const,
  providerModels: (id: string) => ["providers", id, "models"] as const,
  sessions: ["sessions"] as const,
  tools: ["tools"] as const,
  workspaces: ["workspaces"] as const,
};

export const agentsQuery = queryOptions({
  queryKey: queryKeys.agents,
  queryFn: () => client.agents.list() as Promise<Agent[]>,
});

export function agentQuery(id: string) {
  return queryOptions({
    queryKey: queryKeys.agent(id),
    queryFn: () => client.agents.get(id) as Promise<Agent>,
  });
}

export const clientsQuery = queryOptions({
  queryKey: queryKeys.clients,
  queryFn: () => client.clients.list() as Promise<Client[]>,
});

export const providersQuery = queryOptions({
  queryKey: queryKeys.providers,
  queryFn: () => client.providers.list() as Promise<Provider[]>,
});

export function providerModelsQuery(id: string) {
  return queryOptions({
    queryKey: queryKeys.providerModels(id),
    queryFn: async () => {
      try {
        const models = (await client.providers.models.list(id)) as Record<string, ProviderModel>;
        return Object.values(models).sort((a, b) => a.id.localeCompare(b.id));
      } catch {
        return [];
      }
    },
  });
}

export const sessionsQuery = queryOptions({
  queryKey: queryKeys.sessions,
  queryFn: () => client.sessions.list() as Promise<SessionSummary[]>,
});

export const toolsQuery = queryOptions({
  queryKey: queryKeys.tools,
  queryFn: async () => {
    const [tools, mcp, plugins] = await Promise.all([
      client.tools.list() as Promise<ToolsResponse>,
      client.mcp.list() as Promise<MCPResponse>,
      client.plugins.list() as Promise<PluginsResponse>,
    ]);
    return { tools, mcp, plugins };
  },
});

export const workspacesQuery = queryOptions({
  queryKey: queryKeys.workspaces,
  queryFn: () => client.workspaces.list() as Promise<Workspace[]>,
});
