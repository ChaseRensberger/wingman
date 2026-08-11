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
} from "@wingman/core/components/core/alert-dialog";
import { Badge } from "@wingman/core/components/core/badge";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@wingman/core/components/core/table";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { client } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import type { Provider, ProviderModel, ProviderOAuthAttempt } from "@/lib/types";

export const Route = createFileRoute("/providers/$providerId")({
  component: ProviderDetailPage,
});

function ProviderDetailPage() {
  const { providerId } = Route.useParams();
  const [provider, setProvider] = useState<Provider | null>(null);
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [key, setKey] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [oauthAttempt, setOAuthAttempt] = useState<ProviderOAuthAttempt | null>(null);

  async function load() {
    try {
      const providerData = await client.providers.list() as Provider[];
      setProvider(providerData.find((item) => item.id === providerId) ?? null);
      try {
        const modelData = await client.providers.models.list(providerId) as Record<string, ProviderModel>;
        setModels(Object.values(modelData).sort((a, b) => a.id.localeCompare(b.id)));
      } catch {
        setModels([]);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load().catch((err) => showErrorToast(err));
  }, [providerId]);

  useEffect(() => {
    if (!oauthAttempt || oauthAttempt.status !== "pending" || !provider) return;
    const interval = window.setInterval(() => {
      client.providers.oauth.get(provider.id, oauthAttempt.id)
        .then((attempt) => {
          const next = attempt as ProviderOAuthAttempt;
          setOAuthAttempt(next);
          if (next.status === "completed") void load();
        })
        .catch((err) => showErrorToast(err));
    }, 1500);
    return () => window.clearInterval(interval);
  }, [oauthAttempt?.id, oauthAttempt?.status, provider?.id]);

  async function saveKey() {
    if (!provider || !key.trim()) return;
    setSaving(true);
    try {
      await client.providers.auth.set({ providers: { [provider.id]: { type: "api_key", key: key.trim() } } });
      setKey("");
      await load();
    } catch (err) {
      showErrorToast(err);
    } finally {
      setSaving(false);
    }
  }

  async function deleteKey() {
    if (!provider) return;
    setDeleting(true);
    try {
      await client.providers.auth.delete(provider.id);
      await load();
    } catch (err) {
      showErrorToast(err);
    } finally {
      setDeleting(false);
    }
  }

  async function startOAuth(method: "browser" | "device") {
    if (!provider) return;
    try {
      const attempt = await client.providers.oauth.authorize(provider.id, { method }) as ProviderOAuthAttempt;
      setOAuthAttempt(attempt);
      if (method === "browser" && attempt.url) window.open(attempt.url, "_blank", "noopener,noreferrer");
    } catch (err) {
      showErrorToast(err);
    }
  }

  const configured = provider?.auth.configured ?? false;
  const storedKeyConfigured = provider?.auth.source === "stored";
  const supportsApiKey = provider?.auth_types.some((authType) => authType.type === "api_key") ?? false;
  const supportsOAuth = provider?.auth_types.some((authType) => authType.type === "oauth") ?? false;
  const crumbLabel = provider?.name || providerId;

  function formatCost(model: ProviderModel) {
    if (!model.input_cost_per_mtok && !model.output_cost_per_mtok) return "-";
    return `$${model.input_cost_per_mtok ?? 0}/$${model.output_cost_per_mtok ?? 0}`;
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Providers", to: "/providers" }, { label: crumbLabel }]} />
      </div>

      {loading ? (
        <div className="flex items-center gap-3 py-8 text-sm text-muted-foreground">
          <HexWaveSpinner size={24} />
          <span>Loading...</span>
        </div>
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
              <Badge variant={configured || provider.auth.source === "disabled" ? "default" : "secondary"}>
                {configured || provider.auth.source === "disabled" ? "Configured" : "Unconfigured"}
              </Badge>
            </div>
            <div className="grid gap-3 rounded-md border bg-muted/25 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Effective route</div>
                  <div className="mt-1 break-all font-mono text-sm">{provider.route.base_url || "-"}</div>
                </div>
                <Badge variant={provider.route.base_url_source === "config" ? "default" : "ghost"}>
                  {provider.route.base_url_source === "config" ? "Configured URL" : "Catalog URL"}
                </Badge>
              </div>
              <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
                <span>
                  Request auth: <span className="text-foreground">
                    {provider.route.auth_enabled ? "enabled" : "disabled"}
                  </span>
                </span>
                <span>
                  Source: <span className="text-foreground">{provider.route.auth_source}</span>
                </span>
              </div>
            </div>
            {provider.auth.source === "env" && (
              <div className="text-sm text-muted-foreground">
                Using server environment variable {provider.auth.env ? <code>{provider.auth.env}</code> : "for this provider"}.
                Save a key here to override it.
              </div>
            )}
            {provider.auth.source === "disabled" && (
              <div className="text-sm text-muted-foreground">
                Stored and environment credentials are disabled for this provider route by server config.
              </div>
            )}
            {supportsOAuth && (
              <div className="grid gap-2 rounded-md border bg-muted/25 p-3">
                <div>
                  <div className="text-sm font-medium">ChatGPT subscription</div>
                  <div className="text-sm text-muted-foreground">Use ChatGPT Pro or Plus through Codex OAuth.</div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => void startOAuth("browser")} disabled={oauthAttempt?.status === "pending"}>Connect in browser</Button>
                  <Button variant="outline" onClick={() => void startOAuth("device")} disabled={oauthAttempt?.status === "pending"}>Connect headless</Button>
                </div>
                {oauthAttempt && (
                  <div className="text-sm text-muted-foreground">
                    {oauthAttempt.status === "pending" ? oauthAttempt.instructions : oauthAttempt.status === "completed" ? "Connected to ChatGPT." : oauthAttempt.error || "OAuth attempt cancelled."}
                    {oauthAttempt.status === "pending" && oauthAttempt.url && <div className="mt-1 break-all"><a className="text-foreground underline" href={oauthAttempt.url} target="_blank" rel="noreferrer">{oauthAttempt.url}</a></div>}
                  </div>
                )}
              </div>
            )}
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
              {storedKeyConfigured && (
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
                <TableHead>Cost / MTok</TableHead>
                <TableHead>Capabilities</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {models.map((model) => (
                <TableRow key={model.id}>
                  <TableCell className="font-medium">{model.id}</TableCell>
                  <TableCell className="text-muted-foreground">{model.context_window || "-"}</TableCell>
                  <TableCell className="text-muted-foreground">{model.max_output || "-"}</TableCell>
                  <TableCell className="text-muted-foreground">{formatCost(model)}</TableCell>
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
