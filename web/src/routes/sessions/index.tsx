import { useEffect, useRef, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
	CaretRightIcon,
	DotsThreeVerticalIcon,
	FolderOpenIcon,
	MagnifyingGlassIcon,
	PencilSimpleIcon,
	PlusIcon,
	TrashIcon,
	XIcon,
} from "@phosphor-icons/react";

import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
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
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
} from "@/components/core/context-menu";
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
import {
	Empty,
	EmptyActions,
	EmptyDescription,
	EmptyTitle,
} from "@/components/core/empty";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/core/table";
import { wfetch } from "@/lib/client";
import type { Base, Session } from "@/lib/types";
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

type BaseSessions = Record<string, Session[]>;

function latestSession(sessions: Session[]) {
	return sessions
		.slice()
		.sort((a, b) => (b.updated_at || b.created_at).localeCompare(a.updated_at || a.created_at))[0];
}

function displayPath(path: string) {
	if (!path) return "no working directory";
	if (path.length <= 42) return path;
	return `...${path.slice(-39)}`;
}

function SessionsPage() {
	const navigate = useNavigate();
	const [bases, setBases] = useState<Base[]>([]);
	const [sessionsByBase, setSessionsByBase] = useState<BaseSessions>({});
	const [selectedBaseId, setSelectedBaseId] = useState("");
	const [loading, setLoading] = useState(true);
	const [filter, setFilter] = useState("");
	const [filterOpen, setFilterOpen] = useState(false);
	const [creating, setCreating] = useState(false);
	const [deletingSessionId, setDeletingSessionId] = useState("");
	const [deleteSession, setDeleteSession] = useState<Session | null>(null);
	const [editingSession, setEditingSession] = useState<Session | null>(null);
	const [editTitle, setEditTitle] = useState("");
	const [editWorkDir, setEditWorkDir] = useState("");
	const [savingEdit, setSavingEdit] = useState(false);
	const [baseDialogOpen, setBaseDialogOpen] = useState(false);
	const [editingBase, setEditingBase] = useState<Base | null>(null);
	const [baseName, setBaseName] = useState("");
	const [basePath, setBasePath] = useState("");
	const [savingBase, setSavingBase] = useState(false);
	const [deleteBase, setDeleteBase] = useState<Base | null>(null);
	const [deletingBaseId, setDeletingBaseId] = useState("");
	const filterInputRef = useRef<HTMLInputElement>(null);

	const selectedBase = bases.find((base) => base.id === selectedBaseId) ?? null;
	const selectedSessions = selectedBaseId ? sessionsByBase[selectedBaseId] ?? [] : [];
	const filtered = selectedSessions.filter((session) => {
		const haystack = `${session.title || ""} ${session.id}`.toLowerCase();
		return haystack.includes(filter.toLowerCase());
	});

	async function loadBases() {
		const baseData = (await wfetch("/bases")) as Base[];
		const entries = await Promise.all(
			baseData.map(async (base) => {
				const sessionData = (await wfetch(`/bases/${base.id}/sessions`)) as Session[];
				return [base.id, sessionData] as const;
			}),
		);
		setBases(baseData);
		setSessionsByBase(Object.fromEntries(entries));
		return baseData;
	}

	useEffect(() => {
		let cancelled = false;
		async function load() {
			try {
				const baseData = await loadBases();
				if (cancelled) return;
				setSelectedBaseId((current) => current && baseData.some((base) => base.id === current) ? current : "");
			} catch (err) {
				console.error("Failed to load bases", err);
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

	useEffect(() => {
		if (filterOpen) filterInputRef.current?.focus();
	}, [filterOpen]);

	async function handleCreate(baseID = selectedBaseId) {
		if (!baseID) return;
		setCreating(true);
		try {
			const session = (await wfetch("/sessions", {
				method: "POST",
				body: JSON.stringify({ base_id: baseID }),
			})) as Session;
			navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } });
		} catch (err) {
			alert(String(err));
		} finally {
			setCreating(false);
		}
	}

	function openEdit(session: Session) {
		setEditingSession(session);
		setEditTitle(session.title || "");
		setEditWorkDir(session.work_dir || "");
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

	async function handleEdit(e: React.FormEvent) {
		e.preventDefault();
		if (!editingSession) return;
		setSavingEdit(true);
		try {
			const updated = (await wfetch(`/sessions/${editingSession.id}`, {
				method: "PUT",
				body: JSON.stringify({
					title: editTitle.trim(),
					working_directory: editWorkDir.trim(),
				}),
			})) as Session;
			setSessionsByBase((prev) => ({
				...prev,
				[selectedBaseId]: (prev[selectedBaseId] ?? []).map((session) => session.id === updated.id ? updated : session),
			}));
			setEditingSession(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setSavingEdit(false);
		}
	}

	async function handleDelete(session: Session) {
		setDeletingSessionId(session.id);
		try {
			await wfetch(`/sessions/${session.id}`, { method: "DELETE" });
			setSessionsByBase((prev) => ({
				...prev,
				[selectedBaseId]: (prev[selectedBaseId] ?? []).filter((s) => s.id !== session.id),
			}));
			setDeleteSession(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setDeletingSessionId("");
		}
	}

	function openCreateBase() {
		setEditingBase(null);
		setBaseName("");
		setBasePath("");
		setBaseDialogOpen(true);
	}

	function openEditBase(base: Base) {
		setEditingBase(base);
		setBaseName(base.name);
		setBasePath(base.path);
		setBaseDialogOpen(true);
	}

	async function handleSaveBase(e: React.FormEvent) {
		e.preventDefault();
		setSavingBase(true);
		try {
			const payload = { name: baseName.trim(), path: basePath.trim() };
			if (editingBase) {
				const updated = (await wfetch(`/bases/${editingBase.id}`, {
					method: "PUT",
					body: JSON.stringify(payload),
				})) as Base;
				setBases((prev) => prev.map((base) => base.id === updated.id ? updated : base));
			} else {
				const created = (await wfetch("/bases", {
					method: "POST",
					body: JSON.stringify(payload),
				})) as Base;
				setBases((prev) => [created, ...prev]);
				setSessionsByBase((prev) => ({ ...prev, [created.id]: [] }));
				setSelectedBaseId(created.id);
			}
			setBaseDialogOpen(false);
		} catch (err) {
			alert(String(err));
		} finally {
			setSavingBase(false);
		}
	}

	async function handleDeleteBase(base: Base) {
		setDeletingBaseId(base.id);
		try {
			await wfetch(`/bases/${base.id}`, { method: "DELETE" });
			const remaining = await loadBases();
			if (selectedBaseId === base.id) {
				setSelectedBaseId(remaining.find((item) => item.name === "Wingman")?.id || remaining[0]?.id || "");
			}
			setDeleteBase(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setDeletingBaseId("");
		}
	}

	function renderOverview() {
		return (
			<div>
				<div className="mb-5 flex items-center justify-between gap-3">
					<div className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
						Bases <span className="mx-2">·</span> pick one to see its sessions
					</div>
					<Button size="sm" variant="outline" onClick={openCreateBase}>Manage</Button>
				</div>
				<div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
					{bases.map((base, index) => {
						const sessions = sessionsByBase[base.id] ?? [];
						const latest = latestSession(sessions);
						return (
							<div
								key={base.id}
								className={`group relative min-h-56 cursor-pointer rounded-xl border border-border/80 border-l-4 ${baseColors[index % baseColors.length]} bg-card p-5 shadow-sm transition-colors hover:bg-accent/35`}
								onClick={() => setSelectedBaseId(base.id)}
							>
								<div className="flex items-start justify-between gap-3">
									<div className="min-w-0">
										<div className="flex items-center gap-2">
											<span className="size-3 rounded-sm bg-primary/70" />
											<h2 className="truncate font-mono text-base font-semibold tracking-tight">{base.name}</h2>
										</div>
										<p className="mt-5 truncate font-mono text-sm text-muted-foreground">{displayPath(base.path)}</p>
									</div>
									<CaretRightIcon className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
								</div>
								<div className="mt-8">
									<div className="font-mono text-3xl font-semibold leading-none">{sessions.length}</div>
									<div className="mt-1 font-mono text-xs uppercase tracking-[0.22em] text-muted-foreground">
										Sessions{latest ? ` · Last ${timeAgo(latest.updated_at || latest.created_at)}` : ""}
									</div>
								</div>
								<p className="mt-4 truncate font-mono text-sm text-muted-foreground">
									{latest ? `↳ ${latest.title || latest.id}` : "No sessions yet"}
								</p>
								<div className="absolute right-4 bottom-4 flex gap-2">
									<Button size="icon-sm" variant="outline" onClick={(e) => { e.stopPropagation(); openEditBase(base); }} aria-label="Edit base">
										<PencilSimpleIcon className="size-4" />
									</Button>
									<Button size="icon-sm" variant="outline" onClick={(e) => { e.stopPropagation(); handleCreate(base.id); }} aria-label="New session">
										<PlusIcon className="size-4" />
									</Button>
								</div>
							</div>
						);
					})}
					<button
						type="button"
						className="flex min-h-56 items-center justify-center rounded-xl border border-dashed border-border bg-background/40 font-mono text-sm text-muted-foreground transition-colors hover:bg-accent/30 hover:text-foreground"
						onClick={openCreateBase}
					>
						<PlusIcon className="mr-2 size-4" /> New base
					</button>
				</div>
			</div>
		);
	}

	function renderSelectedBase() {
		return (
			<div>
				<div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<div className="min-w-0">
						<div className="flex items-center gap-2">
							<h1 className="truncate text-lg font-semibold">{selectedBase?.name}</h1>
							<Button size="icon-sm" variant="outline" onClick={() => selectedBase && openEditBase(selectedBase)} aria-label="Edit base">
								<PencilSimpleIcon className="size-4" />
							</Button>
							<Button size="icon-sm" variant="outline" onClick={() => selectedBase && setDeleteBase(selectedBase)} aria-label="Delete base">
								<TrashIcon className="size-4" />
							</Button>
						</div>
						<p className="mt-1 truncate text-xs text-muted-foreground">{selectedBase?.path}</p>
					</div>
					<div className="flex items-center gap-3">
						<Button size="sm" onClick={() => handleCreate()} disabled={creating || !selectedBaseId}>
							{creating ? "Creating..." : "New"}
						</Button>
						<div
							className={`flex h-9 items-center rounded-md border bg-card text-muted-foreground shadow-sm transition-all duration-200 focus-within:text-foreground hover:bg-accent hover:text-foreground ${filterOpen || filter ? "w-64 gap-2 px-2" : "w-9 justify-center"}`}
						>
							<Button type="button" variant="ghost" size="icon-xs" className="size-4 shrink-0 rounded-sm p-0" onClick={() => setFilterOpen(true)} aria-label="Filter sessions">
								<MagnifyingGlassIcon className="size-4" />
							</Button>
							<input
								ref={filterInputRef}
								placeholder="Filter sessions..."
								value={filter}
								onChange={(e) => setFilter(e.target.value)}
								tabIndex={filterOpen || filter ? 0 : -1}
								className={`h-7 min-w-0 border-0 bg-transparent p-0 text-sm text-inherit outline-none placeholder:text-muted-foreground ${filterOpen || filter ? "w-full opacity-100" : "w-0 opacity-0"}`}
							/>
							{(filterOpen || filter) && (
								<Button type="button" variant="ghost" size="icon-xs" className="size-4 shrink-0 rounded-sm p-0 text-muted-foreground hover:text-foreground" onClick={() => { setFilter(""); setFilterOpen(false); }} aria-label="Close filter">
									<XIcon className="size-3" />
								</Button>
							)}
						</div>
					</div>
				</div>

				{filtered.length === 0 && filter ? (
					<Empty>
						<EmptyTitle>No sessions found</EmptyTitle>
						<EmptyDescription>Try a different search.</EmptyDescription>
					</Empty>
				) : filtered.length === 0 ? (
					<Empty>
						<EmptyTitle>No sessions in {selectedBase?.name || "this Base"}</EmptyTitle>
						<EmptyDescription>Start a new session from this Base.</EmptyDescription>
						<EmptyActions>
							<Button size="sm" onClick={() => handleCreate()} disabled={creating || !selectedBaseId}>{creating ? "Creating..." : "New"}</Button>
						</EmptyActions>
					</Empty>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Title</TableHead>
								<TableHead>Created</TableHead>
								<TableHead>Workdir</TableHead>
								<TableHead className="w-0"><span className="sr-only">Actions</span></TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{filtered.map((session) => (
								<ContextMenu key={session.id}>
									<ContextMenuTrigger render={<TableRow className="cursor-pointer" onClick={() => navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } })} />}>
										<TableCell className="font-medium">{session.title || session.id}</TableCell>
										<TableCell className="text-muted-foreground">{timeAgo(session.created_at)}</TableCell>
										<TableCell className="max-w-[200px] truncate text-muted-foreground">{session.work_dir || "-"}</TableCell>
										<TableCell className="w-0 text-right">
											<DropdownMenu>
												<DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" onClick={(e) => e.stopPropagation()} aria-label="Session actions" />}>
													<DotsThreeVerticalIcon className="size-4" />
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end" className="w-44">
													<DropdownMenuItem onClick={() => openEdit(session)}><PencilSimpleIcon className="size-4" />Edit session</DropdownMenuItem>
													<DropdownMenuSeparator />
													<DropdownMenuItem variant="destructive" disabled={deletingSessionId === session.id} onClick={() => setDeleteSession(session)}><TrashIcon className="size-4" />{deletingSessionId === session.id ? "Deleting..." : "Delete session"}</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										</TableCell>
									</ContextMenuTrigger>
									<ContextMenuContent className="w-44">
										<ContextMenuItem onClick={() => openEdit(session)}><PencilSimpleIcon className="size-4" />Edit session</ContextMenuItem>
										<ContextMenuSeparator />
										<ContextMenuItem variant="destructive" disabled={deletingSessionId === session.id} onClick={() => setDeleteSession(session)}><TrashIcon className="size-4" />{deletingSessionId === session.id ? "Deleting..." : "Delete session"}</ContextMenuItem>
									</ContextMenuContent>
								</ContextMenu>
							))}
						</TableBody>
					</Table>
				)}
			</div>
		);
	}

	return (
		<div className="mx-auto max-w-[118rem] px-4 py-6">
			<div className="mb-4">
				<PageBreadcrumb items={selectedBase ? [{ label: "Sessions", to: "/sessions" }, { label: selectedBase.name }] : [{ label: "Sessions" }]} />
			</div>

			{loading ? <div className="py-8 text-sm text-muted-foreground">Loading...</div> : selectedBase ? renderSelectedBase() : renderOverview()}

			<Dialog open={baseDialogOpen} onOpenChange={(open) => !open && setBaseDialogOpen(false)}>
				<DialogContent>
					<form onSubmit={handleSaveBase} className="grid gap-4">
						<DialogHeader>
							<DialogTitle>{editingBase ? "Edit Base" : "New Base"}</DialogTitle>
							<DialogDescription>A Base stores a server-side directory path for related sessions.</DialogDescription>
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
							<Button type="submit" disabled={savingBase}>{savingBase ? "Saving..." : "Save Base"}</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<Dialog open={editingSession !== null} onOpenChange={(open) => !open && setEditingSession(null)}>
				<DialogContent>
					<form onSubmit={handleEdit} className="grid gap-4">
						<DialogHeader>
							<DialogTitle>Edit session</DialogTitle>
							<DialogDescription>Change the session name or working directory.</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<div className="grid gap-1"><label className="text-xs font-medium">Name</label><Input placeholder="Session name" value={editTitle} onChange={(e) => setEditTitle(e.target.value)} /></div>
							<div className="grid gap-1">
								<label className="text-xs font-medium">Working directory</label>
								<div className="flex gap-2"><Input placeholder="Optional working directory" value={editWorkDir} onChange={(e) => setEditWorkDir(e.target.value)} /><Button type="button" variant="outline" onClick={() => chooseWorkingDirectory(setEditWorkDir)}><FolderOpenIcon className="size-4" />Choose</Button></div>
								<p className="text-xs text-muted-foreground">Changing this detaches the session from its Base.</p>
							</div>
						</div>
						<DialogFooter><Button type="button" variant="outline" onClick={() => setEditingSession(null)} disabled={savingEdit}>Cancel</Button><Button type="submit" disabled={savingEdit}>{savingEdit ? "Saving..." : "Save changes"}</Button></DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={deleteSession !== null} onOpenChange={(open) => !open && setDeleteSession(null)}>
				<AlertDialogContent>
					<AlertDialogHeader><AlertDialogTitle>Delete session?</AlertDialogTitle><AlertDialogDescription>This will permanently delete {deleteSession?.title || deleteSession?.id}. This action cannot be undone.</AlertDialogDescription></AlertDialogHeader>
					<AlertDialogFooter><AlertDialogCancel disabled={!!deletingSessionId}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={!deleteSession || !!deletingSessionId} onClick={() => deleteSession && handleDelete(deleteSession)}>{deletingSessionId ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<AlertDialog open={deleteBase !== null} onOpenChange={(open) => !open && setDeleteBase(null)}>
				<AlertDialogContent>
					<AlertDialogHeader><AlertDialogTitle>Delete Base?</AlertDialogTitle><AlertDialogDescription>Linked sessions keep their working directories, but they will no longer appear under {deleteBase?.name}.</AlertDialogDescription></AlertDialogHeader>
					<AlertDialogFooter><AlertDialogCancel disabled={!!deletingBaseId}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={!deleteBase || !!deletingBaseId} onClick={() => deleteBase && handleDeleteBase(deleteBase)}>{deletingBaseId ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
