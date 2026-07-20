import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { DesktopIcon, MoonIcon, SunIcon } from "@phosphor-icons/react";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { RadioGroup, RadioGroupItem } from "@wingman/core/components/core/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/core/select";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { type Theme, useTheme } from "@wingman/core/components/theme-provider";
import { getClientId, setClientId, wfetch, type Client } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const { theme, setTheme } = useTheme();
  const [clients, setClients] = useState<Client[]>([]);
  const [activeClientID, setActiveClientID] = useState(getClientId() ?? "");
  const activeClientName = clients.find((client) => client.id === activeClientID)?.name;
  const options = [
    { value: "light", label: "Light", icon: SunIcon },
    { value: "dark", label: "Dark", icon: MoonIcon },
    { value: "system", label: "System", icon: DesktopIcon },
  ] as const;

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
        <Card size="sm">
          <CardHeader>
            <CardTitle>Theme</CardTitle>
          </CardHeader>
          <CardContent>
            <RadioGroup
              value={theme}
              onValueChange={(value) => setTheme(value as Theme)}
              className="inline-grid w-full max-w-md grid-cols-3 rounded-xl border bg-muted/45 p-1"
            >
              {options.map((option) => {
                const Icon = option.icon;
                const active = theme === option.value;
                return (
                  <label
                    key={option.value}
                    className={cn(
                      "flex cursor-pointer items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-all",
                      active
                        ? "bg-background text-foreground shadow-sm ring-1 ring-border/80"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                  >
                    <RadioGroupItem value={option.value} className="sr-only" />
                    <Icon className="size-4" />
                    <span>{option.label}</span>
                  </label>
                );
              })}
            </RadioGroup>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
