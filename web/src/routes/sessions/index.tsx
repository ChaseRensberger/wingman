import { useEffect, useMemo, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
	CheckIcon,
	DotsThreeVerticalIcon,
	FolderIcon,
	FolderOpenIcon,
	MagnifyingGlassIcon,
	PencilSimpleIcon,
	PlusIcon,
	TrashIcon,
	XIcon,
} from "@phosphor-icons/react";

import { PageBreadcrumb } from "@/components/page-breadcrumb";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/core/alert-dialog";
import { Button } from "@/components/core/button";
import { Checkbox } from "@/components/core/checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/core/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/core/dropdown-menu";
import { Input } from "@/components/core/input";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/core/table";
import { wfetch } from "@/lib/client";
import type { Session, Workspace } from "@/lib/types";
import { cn, timeAgo } from "@/lib/utils";

type SessionsSearch = {
	workspace?: string;
};

export const Route = createFileRoute("/sessions/")({
	validateSearch: (search: Record<string, unknown>): SessionsSearch => ({
		workspace: typeof search.workspace === "string" ? search.workspace : undefined,
	}),
	component: SessionsPage,
});

const workspaceColors = [
	"bg-sky-400",
	"bg-emerald-400",
	"bg-blue-400",
	"bg-amber-400",
	"bg-violet-400",
	"bg-pink-400",
];

function displayPath(path: string) {
	if (!path) return "No directory";
	if (path.length <= 56) return path;
	return `...${path.slice(-53)}`;
}

function SessionsPage() {
	const navigate = useNavigate();
	const { workspace: workspaceFilter } = Route.useSearch();
	const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
	const [sessions, setSessions] = useState<Session[]>([]);
	const [loading, setLoading] = useState(true);
	const [creating, setCreating] = useState(false);
	const [search, setSearch] = useState("");

	const [workspaceMenuFilter, setWorkspaceMenuFilter] = useState("");
	const [workspaceDialogOpen, setWorkspaceDialogOpen] = useState(false);
	const [editingWorkspace, setEditingWorkspace] = useState<Workspace | null>(null);
	const [workspaceName, setWorkspaceName] = useState("");
	const [workspacePath, setWorkspacePath] = useState("");
	const [workspaceHasNoDirectory, setWorkspaceHasNoDirectory] = useState(false);
	const [savingWorkspace, setSavingWorkspace] = useState(false);
	const [deleteWorkspace, setDeleteWorkspace] = useState<Workspace | null>(null);
	const [deletingWorkspaceId, setDeletingWorkspaceId] = useState("");

	const [editingSession, setEditingSession] = useState<Session | null>(null);
	const [sessionTitle, setSessionTitle] = useState("");
	const [sessionWorkDir, setSessionWorkDir] = useState("");
	const [savingSession, setSavingSession] = useState(false);
	const [deleteSession, setDeleteSession] = useState<Session | null>(null);
	const [deletingSessionId, setDeletingSessionId] = useState("");

	const workspacesById = useMemo(() => new Map(workspaces.map((workspace) => [workspace.id, workspace])), [workspaces]);
	const selectedWorkspace = workspaceFilter && workspaceFilter !== "none" ? workspacesById.get(workspaceFilter) ?? null : null;
	const workspaceCounts = useMemo(() => {
		const counts = new Map<string, number>();
		for (const session of sessions) {
			if (session.workspace_id) counts.set(session.workspace_id, (counts.get(session.workspace_id) ?? 0) + 1);
		}
		return counts;
	}, [sessions]);
	const noWorkspaceCount = sessions.filter((session) => !session.workspace_id).length;

	const filteredWorkspaces = workspaces.filter((workspace) => {
		const haystack = `${workspace.name} ${workspace.path}`.toLowerCase();
		return haystack.includes(workspaceMenuFilter.toLowerCase());
	});
	const filteredSessions = sessions
		.filter((session) => {
			if (workspaceFilter === "none") return !session.workspace_id;
			if (workspaceFilter) return session.workspace_id === workspaceFilter;
			return true;
		})
		.filter((session) => {
			const workspace = session.workspace_id ? workspacesById.get(session.workspace_id) : null;
			const haystack = `${session.title || ""} ${session.id} ${session.work_dir || ""} ${workspace?.name || ""}`.toLowerCase();
			return haystack.includes(search.toLowerCase());
		})
		.sort((a, b) => (b.updated_at || b.created_at).localeCompare(a.updated_at || a.created_at));

	async function loadData() {
		const [workspaceData, sessionData] = await Promise.all([
			wfetch("/workspaces") as Promise<Workspace[]>,
			wfetch("/sessions") as Promise<Session[]>,
		]);
		setWorkspaces(workspaceData);
		setSessions(sessionData);
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

	function setWorkspaceFilter(workspace?: string) {
		navigate({ to: "/sessions", search: workspace ? { workspace } : {} });
	}

	async function handleCreate() {
		setCreating(true);
		try {
			const session = (await wfetch("/sessions", {
				method: "POST",
				body: JSON.stringify(selectedWorkspace ? { workspace_id: selectedWorkspace.id } : {}),
			})) as Session;
			navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } });
		} catch (err) {
			alert(String(err));
		} finally {
			setCreating(false);
		}
	}

	function openCreateWorkspace() {
		setEditingWorkspace(null);
		setWorkspaceName("");
		setWorkspacePath("");
		setWorkspaceHasNoDirectory(false);
		setWorkspaceDialogOpen(true);
	}

	function openEditWorkspace(workspace: Workspace) {
		setEditingWorkspace(workspace);
		setWorkspaceName(workspace.name);
		setWorkspacePath(workspace.path);
		setWorkspaceHasNoDirectory(workspace.path === "");
		setWorkspaceDialogOpen(true);
	}

	function openEditSession(session: Session) {
		setEditingSession(session);
		setSessionTitle(session.title || "");
		setSessionWorkDir(session.work_dir || "");
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

	async function handleSaveWorkspace(e: React.FormEvent) {
		e.preventDefault();
		setSavingWorkspace(true);
		try {
			const payload = { name: workspaceName.trim(), path: workspaceHasNoDirectory ? "" : workspacePath.trim() };
			let saved: Workspace;
			if (editingWorkspace) {
				saved = (await wfetch(`/workspaces/${editingWorkspace.id}`, {
					method: "PUT",
					body: JSON.stringify(payload),
				})) as Workspace;
			} else {
				saved = (await wfetch("/workspaces", {
					method: "POST",
					body: JSON.stringify(payload),
				})) as Workspace;
				setWorkspaceFilter(saved.id);
			}
			await loadData();
			setWorkspaceDialogOpen(false);
		} catch (err) {
			alert(String(err));
		} finally {
			setSavingWorkspace(false);
		}
	}

	async function handleDeleteWorkspace() {
		if (!deleteWorkspace) return;
		setDeletingWorkspaceId(deleteWorkspace.id);
		try {
			await wfetch(`/workspaces/${deleteWorkspace.id}`, { method: "DELETE" });
			if (workspaceFilter === deleteWorkspace.id) setWorkspaceFilter();
			await loadData();
			setDeleteWorkspace(null);
			setWorkspaceDialogOpen(false);
		} catch (err) {
			alert(String(err));
		} finally {
			setDeletingWorkspaceId("");
		}
	}

	async function handleSaveSession(e: React.FormEvent) {
		e.preventDefault();
		if (!editingSession) return;
		setSavingSession(true);
		try {
			const updated = (await wfetch(`/sessions/${editingSession.id}`, {
				method: "PUT",
				body: JSON.stringify({
					title: sessionTitle.trim(),
					working_directory: sessionWorkDir.trim(),
				}),
			})) as Session;
			setSessions((prev) => prev.map((session) => (session.id === updated.id ? updated : session)));
			setEditingSession(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setSavingSession(false);
		}
	}

	async function handleDeleteSession() {
		if (!deleteSession) return;
		setDeletingSessionId(deleteSession.id);
		try {
			await wfetch(`/sessions/${deleteSession.id}`, { method: "DELETE" });
			setSessions((prev) => prev.filter((session) => session.id !== deleteSession.id));
			setDeleteSession(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setDeletingSessionId("");
		}
	}

	const selectedWorkspaceColor = selectedWorkspace ? workspaceColors[Math.max(0, workspaces.findIndex((workspace) => workspace.id === selectedWorkspace.id)) % workspaceColors.length] : "";
	const createLabel = selectedWorkspace ? `New session in ${selectedWorkspace.name}` : "New session";

	return (
		<div className="mx-auto max-w-[118rem] px-4 py-6">
			<div className="mb-4">
				<PageBreadcrumb items={[{ label: "Sessions" }]} />
			</div>

			{loading ? (
				<div className="py-8 text-sm text-muted-foreground">Loading...</div>
			) : (
				<div className="space-y-4">
					<div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
						<div className="flex flex-col gap-2 sm:flex-row sm:items-center">
							<DropdownMenu>
								<DropdownMenuTrigger render={<Button variant="outline" className="h-10 justify-between gap-2 px-3 font-mono uppercase tracking-[0.16em] text-muted-foreground" />}>
									<span className="text-xs">Workspace</span>
									{selectedWorkspace ? (
										<span className="flex items-center gap-2 normal-case tracking-normal text-foreground">
											<span className={cn("size-2 rounded-sm", selectedWorkspaceColor)} />
											{selectedWorkspace.name}
										</span>
									) : workspaceFilter === "none" ? (
										<span className="normal-case tracking-normal text-foreground">None</span>
									) : (
										<span className="normal-case tracking-normal text-foreground">All</span>
									)}
								</DropdownMenuTrigger>
								<DropdownMenuContent className="w-80 p-2" align="start">
									<Input
										className="mb-2 h-9"
										placeholder="Filter workspaces..."
										value={workspaceMenuFilter}
										onChange={(e) => setWorkspaceMenuFilter(e.target.value)}
									/>
									<DropdownMenuItem className="min-h-9 justify-between" onClick={() => setWorkspaceFilter()}>
										<span className="flex items-center gap-2"><span className="size-2 rounded-sm bg-muted-foreground" />All sessions</span>
										<span className="flex items-center gap-2 text-muted-foreground"><span>{sessions.length}</span>{!workspaceFilter && <CheckIcon className="size-4 text-primary" />}</span>
									</DropdownMenuItem>
									<DropdownMenuItem className="min-h-9 justify-between" onClick={() => setWorkspaceFilter("none")}>
										<span className="flex items-center gap-2 italic text-muted-foreground"><span className="size-2 rounded-sm border border-dashed" />No workspace</span>
										<span className="flex items-center gap-2 text-muted-foreground"><span>{noWorkspaceCount}</span>{workspaceFilter === "none" && <CheckIcon className="size-4 text-primary" />}</span>
									</DropdownMenuItem>
									<DropdownMenuSeparator />
									{filteredWorkspaces.map((workspace, index) => {
										const color = workspaceColors[index % workspaceColors.length];
										return (
											<DropdownMenuItem key={workspace.id} className="min-h-9 justify-between" onClick={() => setWorkspaceFilter(workspace.id)}>
												<span className="flex min-w-0 items-center gap-2">
													<span className={cn("size-2 rounded-sm", color)} />
													<span className="truncate">{workspace.name}</span>
													{!workspace.path && <span className="text-xs italic text-muted-foreground">no directory</span>}
												</span>
												<span className="flex items-center gap-2 text-muted-foreground"><span>{workspaceCounts.get(workspace.id) ?? 0}</span>{workspaceFilter === workspace.id && <CheckIcon className="size-4 text-primary" />}</span>
											</DropdownMenuItem>
										);
									})}
									<DropdownMenuSeparator />
									<DropdownMenuItem className="min-h-9 text-muted-foreground" onClick={openCreateWorkspace}>
										<PlusIcon className="size-4" />New workspace
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
							{selectedWorkspace && (
								<Button variant="ghost" size="icon-sm" onClick={() => setWorkspaceFilter()} aria-label="Clear workspace filter">
									<XIcon className="size-4" />
								</Button>
							)}
							<Button onClick={handleCreate} disabled={creating}>
								<PlusIcon className="size-4" />{creating ? "Creating..." : createLabel}
							</Button>
						</div>

						<div className="relative w-full lg:w-96">
							<MagnifyingGlassIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
							<Input className="h-10 pl-9" placeholder="Search sessions..." value={search} onChange={(e) => setSearch(e.target.value)} />
						</div>
					</div>

					{selectedWorkspace && (
						<div className="flex flex-col gap-3 rounded-lg border bg-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
							<div className="min-w-0 font-mono text-sm">
								<div className="flex min-w-0 items-center gap-2">
									<span className={cn("size-2 rounded-sm", selectedWorkspaceColor)} />
									<span className="font-semibold">{selectedWorkspace.name}</span>
									<span className="truncate text-muted-foreground">{displayPath(selectedWorkspace.path)}</span>
								</div>
							</div>
							<Button variant="outline" size="sm" onClick={() => openEditWorkspace(selectedWorkspace)}>
								<PencilSimpleIcon className="size-4" />Edit
							</Button>
						</div>
					)}

					{filteredSessions.length === 0 ? (
						<div className="rounded-lg border bg-card px-5 py-12 text-center text-sm text-muted-foreground">No sessions found.</div>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Title</TableHead>
									<TableHead>Workspace</TableHead>
									<TableHead>Updated</TableHead>
									<TableHead>Workdir</TableHead>
									<TableHead className="w-0"><span className="sr-only">Actions</span></TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{filteredSessions.map((session) => {
									const workspace = session.workspace_id ? workspacesById.get(session.workspace_id) : undefined;
									const workspaceIndex = workspace ? workspaces.findIndex((item) => item.id === workspace.id) : -1;
									const color = workspaceIndex >= 0 ? workspaceColors[workspaceIndex % workspaceColors.length] : "bg-muted-foreground";
									return (
										<TableRow key={session.id} className="cursor-pointer" onClick={() => navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } })}>
											<TableCell className="font-medium">{session.title || session.id}</TableCell>
											<TableCell className="max-w-[420px]">
												{workspace ? (
													<div className="flex min-w-0 items-center gap-2">
														<span className={cn("size-2 rounded-sm", color)} />
														<span className="font-medium">{workspace.name}</span>
														<span className="truncate text-muted-foreground">{displayPath(workspace.path)}</span>
													</div>
												) : (
													<span className="italic text-muted-foreground">No workspace</span>
												)}
											</TableCell>
											<TableCell className="whitespace-nowrap text-muted-foreground">{timeAgo(session.updated_at || session.created_at)}</TableCell>
											<TableCell className="max-w-[320px] truncate text-muted-foreground">{session.work_dir || "-"}</TableCell>
											<TableCell className="w-0 text-right" onClick={(e) => e.stopPropagation()}>
												<DropdownMenu>
													<DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label="Session actions" />}>
														<DotsThreeVerticalIcon className="size-4" />
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end" className="w-44">
														<DropdownMenuItem onClick={() => openEditSession(session)}><PencilSimpleIcon className="size-4" />Edit session</DropdownMenuItem>
														<DropdownMenuSeparator />
														<DropdownMenuItem variant="destructive" onClick={() => setDeleteSession(session)}><TrashIcon className="size-4" />Delete session</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											</TableCell>
										</TableRow>
									);
								})}
							</TableBody>
						</Table>
					)}
				</div>
			)}

			<Dialog open={workspaceDialogOpen} onOpenChange={(open) => !open && setWorkspaceDialogOpen(false)}>
				<DialogContent className="sm:max-w-2xl">
					<form onSubmit={handleSaveWorkspace} className="grid gap-4">
						<DialogHeader>
							<DialogTitle className="flex items-center gap-2"><FolderIcon className="size-4" />{editingWorkspace ? "Edit workspace" : "New workspace"}</DialogTitle>
							<DialogDescription>A workspace is an optional saved context for creating and filtering sessions.</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<div className="grid gap-1">
								<label className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">Name</label>
								<Input value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} placeholder="e.g. wingman" required />
							</div>
							<div className="grid gap-2">
								<div className="flex items-center justify-between gap-3">
									<label className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">Working directory</label>
									<label className="flex items-center gap-2 text-xs text-muted-foreground">
										<Checkbox checked={workspaceHasNoDirectory} onCheckedChange={(checked) => setWorkspaceHasNoDirectory(checked === true)} />
										No directory
									</label>
								</div>
								<div className="flex gap-2">
									<Input value={workspacePath} onChange={(e) => setWorkspacePath(e.target.value)} placeholder="/path/to/project" disabled={workspaceHasNoDirectory} />
									<Button type="button" variant="outline" onClick={() => chooseWorkingDirectory(setWorkspacePath)} disabled={workspaceHasNoDirectory}><FolderOpenIcon className="size-4" />Choose</Button>
								</div>
								<p className="text-xs text-muted-foreground">Sessions created in this workspace will not start with a working directory.</p>
							</div>
						</div>
						<DialogFooter className="items-center sm:justify-between">
							{editingWorkspace ? (
								<Button type="button" variant="outline" className="text-destructive hover:text-destructive" onClick={() => setDeleteWorkspace(editingWorkspace)} disabled={savingWorkspace}>
									<TrashIcon className="size-4" />Delete
								</Button>
							) : <span />}
							<div className="flex gap-2">
								<Button type="button" variant="outline" onClick={() => setWorkspaceDialogOpen(false)} disabled={savingWorkspace}>Cancel</Button>
								<Button type="submit" disabled={savingWorkspace}>{savingWorkspace ? "Saving..." : editingWorkspace ? "Save" : "Create"}</Button>
							</div>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<Dialog open={editingSession !== null} onOpenChange={(open) => !open && setEditingSession(null)}>
				<DialogContent>
					<form onSubmit={handleSaveSession} className="grid gap-4">
						<DialogHeader>
							<DialogTitle>Edit session</DialogTitle>
							<DialogDescription>Change the session name or working directory. Editing the directory removes any workspace link.</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<div className="grid gap-1"><label className="text-xs font-medium">Name</label><Input placeholder="Session name" value={sessionTitle} onChange={(e) => setSessionTitle(e.target.value)} /></div>
							<div className="grid gap-1">
								<label className="text-xs font-medium">Working directory</label>
								<div className="flex gap-2"><Input placeholder="Optional working directory" value={sessionWorkDir} onChange={(e) => setSessionWorkDir(e.target.value)} /><Button type="button" variant="outline" onClick={() => chooseWorkingDirectory(setSessionWorkDir)}><FolderOpenIcon className="size-4" />Choose</Button></div>
								<p className="text-xs text-muted-foreground">Clear this field to remove the working directory.</p>
							</div>
						</div>
						<DialogFooter><Button type="button" variant="outline" onClick={() => setEditingSession(null)} disabled={savingSession}>Cancel</Button><Button type="submit" disabled={savingSession}>{savingSession ? "Saving..." : "Save changes"}</Button></DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={deleteWorkspace !== null} onOpenChange={(open) => !open && setDeleteWorkspace(null)}>
				<AlertDialogContent>
					<AlertDialogHeader><AlertDialogTitle>Delete workspace?</AlertDialogTitle><AlertDialogDescription>Linked sessions keep their working directories, but they will no longer be linked to {deleteWorkspace?.name}.</AlertDialogDescription></AlertDialogHeader>
					<AlertDialogFooter><AlertDialogCancel disabled={!!deletingWorkspaceId}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={!deleteWorkspace || !!deletingWorkspaceId} onClick={handleDeleteWorkspace}>{deletingWorkspaceId ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<AlertDialog open={deleteSession !== null} onOpenChange={(open) => !open && setDeleteSession(null)}>
				<AlertDialogContent>
					<AlertDialogHeader><AlertDialogTitle>Delete session?</AlertDialogTitle><AlertDialogDescription>This will permanently delete {deleteSession?.title || deleteSession?.id}. This action cannot be undone.</AlertDialogDescription></AlertDialogHeader>
					<AlertDialogFooter><AlertDialogCancel disabled={!!deletingSessionId}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={!deleteSession || !!deletingSessionId} onClick={handleDeleteSession}>{deletingSessionId ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
