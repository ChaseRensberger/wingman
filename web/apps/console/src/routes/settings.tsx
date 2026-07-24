import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/core/select";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { getClientId, setClientId, wfetch, type Client } from "@/lib/client";
import { getDisplayName, setDisplayName } from "@/lib/greeting";
import { showErrorToast } from "@/lib/toast";
import { Input } from "@wingman/core/components/core/input";
import { ThemePreviewSwitcher } from "@wingman/core/components/theme-preview-switcher";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const [clients, setClients] = useState<Client[]>([]);
  const [activeClientID, setActiveClientID] = useState(getClientId() ?? "");
  const [displayName, setDisplayNameInput] = useState(getDisplayName());
  const activeClientName = clients.find((client) => client.id === activeClientID)?.name;

  useEffect(() => {
    let cancelled = false;
    async function loadClients() {
      try {
        const data = (await wfetch("/clients")) as Client[];
        if (!cancelled) setClients(data.sort((a, b) => a.name.localeCompare(b.name)));
      } catch (err) {
        showErrorToast(err);
      }
    }
    loadClients();
    return () => {
      cancelled = true;
    };
  }, []);

  function selectClient(clientID: string | null) {
    if (!clientID) return;
    setClientId(clientID);
    setActiveClientID(clientID);
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Settings" }]} />
      </div>

      <div className="space-y-4">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Display name</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Input
              className="max-w-md"
              value={displayName}
              onChange={(event) => {
                setDisplayNameInput(event.target.value);
                setDisplayName(event.target.value);
              }}
              placeholder="Your name"
            />
            <p className="text-sm text-muted-foreground">Used for personalized greetings when starting a new session. Leave blank to stay incognito.</p>
          </CardContent>
        </Card>
        <Card size="sm">
          <CardHeader>
            <CardTitle>Active client</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Select value={activeClientID} onValueChange={selectClient}>
              <SelectTrigger className="max-w-md">
                <SelectValue placeholder="Select a client">{activeClientName}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {clients.map((client) => <SelectItem key={client.id} value={client.id}>{client.name}</SelectItem>)}
              </SelectContent>
            </Select>
            <p className="text-sm text-muted-foreground">Sessions and Workspaces are scoped to this client. New installations default to the console's <code>web</code> client.</p>
          </CardContent>
        </Card>
        <ThemePreviewSwitcher />
      </div>
    </div>
  );
}
