import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@wingman/core/components/core/button";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { getDisplayName, setDisplayName } from "@/lib/greeting";
import { Input } from "@wingman/core/components/core/input";
import { ThemePreviewSwitcher } from "@wingman/core/components/theme-preview-switcher";
import { client } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function SettingsPage() {
	const [displayName, setDisplayNameInput] = useState(getDisplayName());
	return <div className="mx-auto max-w-5xl px-4 py-6">
		<div className="mb-4"><PageBreadcrumb items={[{ label: "Settings" }]} /></div>
		<div className="space-y-4">
			<ClientManagement />
			<Card size="sm"><CardHeader><CardTitle>Console display name</CardTitle></CardHeader><CardContent className="space-y-2"><Input className="max-w-md" value={displayName} onChange={(event) => { setDisplayNameInput(event.target.value); setDisplayName(event.target.value); }} placeholder="Your name" /><p className="text-sm text-muted-foreground">Used for personalized greetings when starting a new session.</p></CardContent></Card>
			<ThemePreviewSwitcher />
		</div>
	</div>;
}

type Client = { id: string; name: string };
function ClientManagement() {
	const [clients, setClients] = useState<Client[]>([]);
	const [clientName, setClientName] = useState("");
	const [clientID, setClientID] = useState("");
	const [busy, setBusy] = useState(false);

	async function load() {
		try {
			setClients(await client.clients.list() as Client[]);
		} catch { }
	}
	useEffect(() => { void load(); }, []);

	async function createClient() {
		if (!clientID.trim() || !clientName.trim()) return;
		setBusy(true);
		try {
			const created = await client.clients.create({ id: clientID.trim(), name: clientName.trim() }) as { client: Client };
			setClients((current) => [created.client, ...current]); setClientID(""); setClientName("");
		} catch (err) { showErrorToast(err); } finally { setBusy(false); }
	}

	return <Card size="sm"><CardHeader><CardTitle>Clients</CardTitle></CardHeader><CardContent className="space-y-4">
		<p className="text-sm text-muted-foreground">Named clients organize sessions and workspaces. They do not control access to Wingman.</p>
		<>
			<div className="flex flex-wrap gap-2"><Input value={clientID} onChange={(event) => setClientID(event.target.value)} placeholder="Client ID (cli_reference)" /><Input value={clientName} onChange={(event) => setClientName(event.target.value)} placeholder="Display name" /><Button size="sm" disabled={busy || !clientID.trim() || !clientName.trim()} onClick={createClient}>Create client</Button></div>
			<div className="space-y-1 text-sm">{clients.map((client) => <p key={client.id}>{client.name} <span className="font-mono text-muted-foreground">{client.id}</span></p>)}</div>
		</>
	</CardContent></Card>;
}
