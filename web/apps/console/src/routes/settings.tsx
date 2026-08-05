import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@wingman/core/components/core/button";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { getDisplayName, setDisplayName } from "@/lib/greeting";
import { Input } from "@wingman/core/components/core/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@wingman/core/components/core/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@wingman/core/components/core/table";
import { ThemePreviewSwitcher } from "@wingman/core/components/theme-preview-switcher";
import { APIError, api, apiData, rotateClientToken } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function SettingsPage() {
  const [displayName, setDisplayNameInput] = useState(getDisplayName());
  return <div className="mx-auto max-w-5xl px-4 py-6">
    <div className="mb-4"><PageBreadcrumb items={[{ label: "Settings" }]} /></div>
    <div className="space-y-4">
      <ClientManagement />
      <Card size="sm"><CardHeader><CardTitle>Console display name</CardTitle></CardHeader><CardContent className="space-y-2"><Input className="max-w-md" value={displayName} onChange={(event) => { setDisplayNameInput(event.target.value); setDisplayName(event.target.value); }} placeholder="Your name" /><p className="text-sm text-muted-foreground">Used for personalized greetings when starting a new session. Leave blank to stay incognito.</p></CardContent></Card>
      <ThemePreviewSwitcher />
    </div>
  </div>;
}

type Client = { id: string; name: string };
type AuthSession = { id: string; client_id: string; owner: boolean; created_at: string; expires_at?: string; revoked_at?: string };

function ClientManagement() {
  const [clients, setClients] = useState<Client[]>([]);
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [selectedClient, setSelectedClient] = useState("");
  const [clientName, setClientName] = useState("");
  const [clientID, setClientID] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function load() {
    try {
      const [clientData, sessionData] = await Promise.all([apiData(api.GET("/clients")) as Promise<Client[]>, apiData(api.GET("/auth/sessions")) as Promise<AuthSession[]>]);
      setClients(clientData); setSessions(sessionData); setSelectedClient((current) => current || clientData[0]?.id || ""); setError("");
    } catch (err) { setError(err instanceof APIError && err.status === 401 ? "Client management is available from the local owner Console." : String(err)); }
  }
  useEffect(() => { void load(); }, []);

  async function createClient() {
    if (!clientID.trim() || !clientName.trim()) return;
    setBusy(true);
    try {
      const created = await apiData(api.POST("/clients", { body: { id: clientID.trim(), name: clientName.trim() } })) as { client: Client; session: AuthSession; token: string };
      setClients((current) => [created.client, ...current]); setSessions((current) => [created.session, ...current]); setSelectedClient(created.client.id); setToken(created.token); setClientID(""); setClientName("");
    } catch (err) { showErrorToast(err); } finally { setBusy(false); }
  }
  async function rotateToken() {
    if (!selectedClient) return;
    setBusy(true);
    try { const value = await rotateClientToken(selectedClient); setToken(value.token); await load(); }
    catch (err) { showErrorToast(err); } finally { setBusy(false); }
  }
  async function revoke(session: AuthSession) {
    setBusy(true);
    try { await apiData(api.DELETE("/auth/sessions/{id}", { params: { path: { id: session.id } } })); await load(); }
    catch (err) { showErrorToast(err); } finally { setBusy(false); }
  }

  return <Card size="sm"><CardHeader><CardTitle>Clients</CardTitle></CardHeader><CardContent className="space-y-4">
    <p className="text-sm text-muted-foreground">Create named clients, copy their access token once, and revoke their sessions.</p>
    {error ? <p className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p> : <>
      <div className="flex flex-wrap gap-2"><Input value={clientID} onChange={(event) => setClientID(event.target.value)} placeholder="Client ID (cli_reference)" /><Input value={clientName} onChange={(event) => setClientName(event.target.value)} placeholder="Display name" /><Button size="sm" disabled={busy || !clientID.trim() || !clientName.trim()} onClick={createClient}>Create client</Button></div>
      <div className="flex flex-wrap items-center gap-2"><Select value={selectedClient} onValueChange={(value) => setSelectedClient(value ?? "")}><SelectTrigger className="min-w-52"><SelectValue placeholder="Select client" /></SelectTrigger><SelectContent>{clients.map((client) => <SelectItem key={client.id} value={client.id}>{client.name} ({client.id})</SelectItem>)}</SelectContent></Select><Button size="sm" variant="outline" disabled={busy || !selectedClient} onClick={rotateToken}>Rotate access token</Button></div>
      {token ? <div className="rounded-md border bg-muted/40 p-3"><p className="mb-1 text-xs text-muted-foreground">Copy this value now. Wingman will not show it again.</p><code className="break-all text-sm">{token}</code></div> : null}
      <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>Client</TableHead><TableHead>Created</TableHead><TableHead>Expires</TableHead><TableHead>Actions</TableHead></TableRow></TableHeader><TableBody>{sessions.map((session) => <TableRow key={session.id}><TableCell><div>{session.owner ? "Owner Console" : session.client_id}</div><div className="font-mono text-xs text-muted-foreground">{session.id}</div></TableCell><TableCell>{formatDate(session.created_at)}</TableCell><TableCell>{formatDate(session.expires_at)}</TableCell><TableCell><Button size="xs" variant="outline" disabled={busy || Boolean(session.revoked_at)} onClick={() => revoke(session)}>{session.revoked_at ? "Revoked" : "Revoke"}</Button></TableCell></TableRow>)}{sessions.length === 0 ? <TableRow><TableCell colSpan={4} className="text-sm text-muted-foreground">No auth sessions.</TableCell></TableRow> : null}</TableBody></Table></div>
    </>}
  </CardContent></Card>;
}

function formatDate(value?: string) { if (!value) return "-"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString(); }
