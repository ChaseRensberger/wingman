import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/core/alert-dialog";
import { Badge } from "@/components/core/badge";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/core/table";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { wfetch } from "@/lib/client";
import type { Provider, ProviderAuthResponse, ProviderModel } from "@/lib/types";

export const Route = createFileRoute("/providers/$providerId")({
  component: ProviderDetailPage,
});

function ProviderDetailPage() {
  const { providerId } = Route.useParams();
  const [provider, setProvider] = useState<Provider | null>(null);
  const [auth, setAuth] = useState<ProviderAuthResponse>({ providers: {} });
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [key, setKey] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);

  async function load() {
    try {
      const [providerData, authData] = await Promise.all([
        wfetch("/provider") as Promise<Provider[]>,
        wfetch("/provider/auth") as Promise<ProviderAuthResponse>,
      ]);
      setProvider(providerData.find((item) => item.id === providerId) ?? null);
      setAuth(authData);
      try {
        const modelData = (await wfetch(`/provider/${providerId}/models`)) as Record<string, ProviderModel>;
        setModels(Object.values(modelData).sort((a, b) => a.id.localeCompare(b.id)));
      } catch {
        setModels([]);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load().catch((err) => alert(String(err)));
  }, [providerId]);

  async function saveKey() {
    if (!provider || !key.trim()) return;
    setSaving(true);
    try {
      await wfetch("/provider/auth", {
        method: "PUT",
        body: JSON.stringify({ providers: { [provider.id]: { type: "api_key", key: key.trim() } } }),
      });
      setKey("");
      await load();
    } catch (err) {
      alert(String(err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteKey() {
    if (!provider) return;
    setDeleting(true);
    try {
      await wfetch(`/provider/auth/${provider.id}`, { method: "DELETE" });
      await load();
    } catch (err) {
      alert(String(err));
    } finally {
      setDeleting(false);
    }
  }

  const configured = provider ? auth.providers[provider.id]?.configured : false;
  const supportsApiKey = provider?.auth_types.some((authType) => authType.type === "api_key") ?? false;
  const crumbLabel = provider?.name || providerId;

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Providers", to: "/providers" }, { label: crumbLabel }]} />
      </div>

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : !provider ? (
        <div className="py-8 text-sm text-muted-foreground">Provider not found.</div>
      ) : (
        <div className="grid gap-4">
          <div className="grid gap-4 rounded-lg border bg-card p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="text-sm font-medium">{provider.name}</div>
                <div className="font-mono text-xs text-muted-foreground">{provider.id}</div>
              </div>
              <Badge variant={configured ? "default" : "secondary"}>{configured ? "Configured" : "Needs key"}</Badge>
            </div>
            <div className="grid gap-2 sm:grid-cols-[1fr_auto_auto]">
              <Input
                type="password"
                value={key}
                placeholder={configured ? "New API key" : "API key"}
                onChange={(e) => setKey(e.target.value)}
                disabled={!supportsApiKey}
              />
              <Button onClick={saveKey} disabled={saving || !supportsApiKey || !key.trim()}>
                {saving ? "Saving..." : configured ? "Replace key" : "Save key"}
              </Button>
              {configured && (
                <AlertDialog>
                  <AlertDialogTrigger render={<Button variant="destructive" disabled={deleting} />}>
                    {deleting ? "Deleting..." : "Delete key"}
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Delete API key?</AlertDialogTitle>
                      <AlertDialogDescription>
                        This will remove the saved API key for {provider.name}. You will need to enter a new key before using this provider again.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
                      <AlertDialogAction variant="destructive" onClick={deleteKey} disabled={deleting}>
                        {deleting ? "Deleting..." : "Delete key"}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              )}
            </div>
            {!supportsApiKey && (
              <div className="text-sm text-muted-foreground">This provider does not support API key auth.</div>
            )}
          </div>

          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Model</TableHead>
                <TableHead>Context</TableHead>
                <TableHead>Output</TableHead>
                <TableHead>Capabilities</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {models.map((model) => (
                <TableRow key={model.id}>
                  <TableCell className="font-medium">{model.id}</TableCell>
                  <TableCell className="text-muted-foreground">{model.context_window || "-"}</TableCell>
                  <TableCell className="text-muted-foreground">{model.max_output || "-"}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {model.tools && <Badge variant="outline">tools</Badge>}
                      {model.images && <Badge variant="outline">images</Badge>}
                      {model.reasoning && <Badge variant="outline">reasoning</Badge>}
                      {model.structured_output && <Badge variant="outline">structured</Badge>}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
