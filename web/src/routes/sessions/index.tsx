import { useEffect, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { CaretRightIcon, FolderOpenIcon, PencilSimpleIcon, PlusIcon } from "@phosphor-icons/react";

import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { Badge } from "@/components/core/badge";
import { Button } from "@/components/core/button";
import {
	Card,
	CardAction,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/core/card";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/core/dialog";
import { Input } from "@/components/core/input";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/core/table";
import { workspaceSlug } from "@/lib/workspace-slug";
import { wfetch } from "@/lib/client";
import type { Workspace, Session } from "@/lib/types";
import { timeAgo } from "@/lib/utils";

export const Route = createFileRoute("/sessions/")({
	component: SessionsPage,
});

const baseColors = [
	"border-l-sky-400",
	"border-l-emerald-400",
	"border-l-blue-400",
	"border-l-amber-400",
	"border-l-violet-400",
	"border-l-pink-400",
];

type WorkspaceSessions = Record<string, Session[]>;

function latestSession(sessions: Session[]) {
	return sessions
		.slice()
		.sort((a, b) => (b.updated_at || b.created_at).localeCompare(a.updated_at || a.created_at))[0];
}

function displayPath(path: string) {
	if (!path) return "no working directory";
	if (path.length <= 48) return path;
	return `...${path.slice(-45)}`;
}

function SessionsPage() {
	const navigate = useNavigate();
	const [workspaces, setBases] = useState<Workspace[]>([]);
	const [sessions, setSessions] = useState<Session[]>([]);
	const [sessionsByBase, setSessionsByBase] = useState<WorkspaceSessions>({});
	const [loading, setLoading] = useState(true);
	const [creating, setCreating] = useState(false);
	const [baseDialogOpen, setBaseDialogOpen] = useState(false);
	const [editingBase, setEditingBase] = useState<Workspace | null>(null);
	const [baseName, setBaseName] = useState("");
	const [basePath, setBasePath] = useState("");
	const [savingBase, setSavingBase] = useState(false);

	const workspacesById = new Map(workspaces.map((workspace) => [workspace.id, workspace]));
	const recentSessions = sessions
		.slice()
		.sort((a, b) => (b.updated_at || b.created_at).localeCompare(a.updated_at || a.created_at))
		.slice(0, 8);

	async function loadData() {
		const [baseData, sessionData] = await Promise.all([
			wfetch("/workspaces") as Promise<Workspace[]>,
			wfetch("/sessions") as Promise<Session[]>,
		]);
		const entries = await Promise.all(
			baseData.map(async (workspace) => {
				const baseSessions = (await wfetch(`/workspaces/${workspace.id}/sessions`)) as Session[];
				return [workspace.id, baseSessions] as const;
			}),
		);
		setBases(baseData);
		setSessions(sessionData);
		setSessionsByBase(Object.fromEntries(entries));
	}

	useEffect(() => {
		let cancelled = false;
		async function load() {
			try {
				await loadData();
			} catch (err) {
				console.error("Failed to load sessions", err);
				alert(String(err));
			} finally {
				if (!cancelled) setLoading(false);
			}
		}
		load();
		return () => {
			cancelled = true;
		};
	}, []);

	async function handleCreate(workspace?: Workspace) {
		setCreating(true);
		try {
			const session = (await wfetch("/sessions", {
				method: "POST",
				body: JSON.stringify(workspace ? { workspace_id: workspace.id } : {}),
			})) as Session;
			navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } });
		} catch (err) {
			alert(String(err));
		} finally {
			setCreating(false);
		}
	}

	function openCreateWorkspace() {
		setEditingBase(null);
		setBaseName("");
		setBasePath("");
		setBaseDialogOpen(true);
	}

	function openEditBase(workspace: Workspace) {
		setEditingBase(workspace);
		setBaseName(workspace.name);
		setBasePath(workspace.path);
		setBaseDialogOpen(true);
	}

	async function chooseWorkingDirectory(setter: (path: string) => void) {
		const picker = (window as Window & {
			showDirectoryPicker?: () => Promise<{ name: string }>;
		}).showDirectoryPicker;
		if (!picker) {
			alert("This browser does not support directory picking. Enter the path manually.");
			return;
		}
		try {
			const handle = await picker.call(window);
			setter(handle.name);
		} catch (err) {
			if ((err as Error).name !== "AbortError") alert(String(err));
		}
	}

	async function handleSaveBase(e: React.FormEvent) {
		e.preventDefault();
		setSavingBase(true);
		try {
			const payload = { name: baseName.trim(), path: basePath.trim() };
			if (editingBase) {
				await wfetch(`/workspaces/${editingBase.id}`, {
					method: "PUT",
					body: JSON.stringify(payload),
				});
			} else {
				const created = (await wfetch("/workspaces", {
					method: "POST",
					body: JSON.stringify(payload),
				})) as Workspace;
				navigate({ to: "/sessions/workspaces/$workspaceSlug", params: { workspaceSlug: workspaceSlug(created) } });
			}
			await loadData();
			setBaseDialogOpen(false);
		} catch (err) {
			alert(String(err));
		} finally {
			setSavingBase(false);
		}
	}

	return (
		<div className="mx-auto max-w-[118rem] px-4 py-6">
			<div className="mb-4">
				<PageBreadcrumb items={[{ label: "Sessions" }]} />
			</div>

			{loading ? (
				<div className="py-8 text-sm text-muted-foreground">Loading...</div>
			) : (
				<div className="space-y-8">
					<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
						<div>
							<h1 className="text-2xl font-semibold tracking-tight">Sessions</h1>
							<p className="mt-1 text-sm text-muted-foreground">
								Start anywhere, or use a Workspace when the work belongs to a project directory.
							</p>
						</div>
						<div className="flex gap-2">
							<Button variant="outline" onClick={openCreateWorkspace}>
								<PlusIcon className="size-4" /> New Workspace
							</Button>
							<Button onClick={() => handleCreate()} disabled={creating}>
								{creating ? "Creating..." : "New Session"}
							</Button>
						</div>
					</div>

					<section className="space-y-3">
						<div className="flex items-center justify-between">
							<div className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">Workspaces</div>
						</div>
						<div className="grid grid-cols-[repeat(auto-fill,18rem)] gap-4">
							{workspaces.map((workspace, index) => {
								const baseSessions = sessionsByBase[workspace.id] ?? [];
								const latest = latestSession(baseSessions);
								return (
									<Card
										key={workspace.id}
										size="sm"
										className={`group h-56 w-72 cursor-pointer border-l-4 ${baseColors[index % baseColors.length]} transition-colors hover:bg-accent/35`}
										onClick={() => navigate({ to: "/sessions/workspaces/$workspaceSlug", params: { workspaceSlug: workspaceSlug(workspace) } })}
									>
										<CardHeader>
											<div className="min-w-0">
												<CardTitle className="truncate font-mono">{workspace.name}</CardTitle>
												<CardDescription className="mt-2 truncate font-mono">{displayPath(workspace.path)}</CardDescription>
											</div>
											<CardAction>
												<CaretRightIcon className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
											</CardAction>
										</CardHeader>
										<CardContent className="mt-auto space-y-3">
											<div>
												<div className="font-mono text-3xl font-semibold leading-none">{baseSessions.length}</div>
												<div className="mt-1 font-mono text-xs uppercase tracking-[0.22em] text-muted-foreground">
													Sessions{latest ? ` · Last ${timeAgo(latest.updated_at || latest.created_at)}` : ""}
												</div>
											</div>
											<div className="flex items-center justify-between gap-2">
												<p className="min-w-0 truncate font-mono text-sm text-muted-foreground">
													{latest ? `↳ ${latest.title || latest.id}` : "No sessions yet"}
												</p>
												<div className="flex shrink-0 gap-2">
													<Button size="icon-sm" variant="outline" onClick={(e) => { e.stopPropagation(); openEditBase(workspace); }} aria-label="Edit workspace">
														<PencilSimpleIcon className="size-4" />
													</Button>
													<Button size="icon-sm" variant="outline" disabled={creating} onClick={(e) => { e.stopPropagation(); handleCreate(workspace); }} aria-label="New session in workspace">
														<PlusIcon className="size-4" />
													</Button>
												</div>
											</div>
										</CardContent>
									</Card>
								);
							})}
							<Card
								size="sm"
								className="flex h-56 w-72 cursor-pointer items-center justify-center border-dashed bg-background/40 text-muted-foreground transition-colors hover:bg-accent/30 hover:text-foreground"
								onClick={openCreateWorkspace}
							>
								<div className="font-mono text-sm"><PlusIcon className="mr-2 inline size-4" />New workspace</div>
							</Card>
						</div>
					</section>

					<section className="space-y-3">
						<div className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">Recent Sessions</div>
						{recentSessions.length === 0 ? (
							<div className="py-8 text-sm text-muted-foreground">No sessions yet.</div>
						) : (
							<Table>
								<TableHeader>
									<TableRow>
										<TableHead>Title</TableHead>
										<TableHead>Workspace</TableHead>
										<TableHead>Updated</TableHead>
										<TableHead>Workdir</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{recentSessions.map((session) => {
										const workspace = session.workspace_id ? workspacesById.get(session.workspace_id) : undefined;
										return (
											<TableRow key={session.id} className="cursor-pointer" onClick={() => navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } })}>
												<TableCell className="font-medium">{session.title || session.id}</TableCell>
												<TableCell>{workspace ? <Badge variant="ghost">{workspace.name}</Badge> : <span className="text-muted-foreground">-</span>}</TableCell>
												<TableCell className="text-muted-foreground">{timeAgo(session.updated_at || session.created_at)}</TableCell>
												<TableCell className="max-w-[320px] truncate text-muted-foreground">{session.work_dir || "-"}</TableCell>
											</TableRow>
										);
									})}
								</TableBody>
							</Table>
						)}
					</section>
				</div>
			)}

			<Dialog open={baseDialogOpen} onOpenChange={(open) => !open && setBaseDialogOpen(false)}>
				<DialogContent>
					<form onSubmit={handleSaveBase} className="grid gap-4">
						<DialogHeader>
							<DialogTitle>{editingBase ? "Edit Workspace" : "New Workspace"}</DialogTitle>
							<DialogDescription>A Workspace stores a server-side directory path for related sessions.</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<div className="grid gap-1">
								<label className="text-xs font-medium">Name</label>
								<Input value={baseName} onChange={(e) => setBaseName(e.target.value)} placeholder="Wingman" required />
							</div>
							<div className="grid gap-1">
								<label className="text-xs font-medium">Path</label>
								<div className="flex gap-2">
									<Input value={basePath} onChange={(e) => setBasePath(e.target.value)} placeholder="/home/chase/Projects/wingman" required />
									<Button type="button" variant="outline" onClick={() => chooseWorkingDirectory(setBasePath)}><FolderOpenIcon className="size-4" />Choose</Button>
								</div>
								<p className="text-xs text-muted-foreground">Path must exist on the Wingman server.</p>
							</div>
						</div>
						<DialogFooter>
							<Button type="button" variant="outline" onClick={() => setBaseDialogOpen(false)} disabled={savingBase}>Cancel</Button>
							<Button type="submit" disabled={savingBase}>{savingBase ? "Saving..." : "Save Workspace"}</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>
		</div>
	);
}
