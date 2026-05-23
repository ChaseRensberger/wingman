import { useEffect, useRef, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { wfetch } from "@/lib/client";
import type { Session } from "@/lib/types";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import {
	Table,
	TableHeader,
	TableBody,
	TableRow,
	TableHead,
	TableCell,
} from "@/components/core/table";
import {
	Empty,
	EmptyTitle,
	EmptyDescription,
	EmptyActions,
} from "@/components/core/empty";
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
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/core/dropdown-menu";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/core/dialog";
import {
	DotsThreeVerticalIcon,
	FolderOpenIcon,
	MagnifyingGlassIcon,
	PencilSimpleIcon,
	PlusIcon,
	TrashIcon,
	XIcon,
} from "@phosphor-icons/react";

import { timeAgo } from "@/lib/utils";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const Route = createFileRoute("/sessions/")({
	component: SessionsPage,
});

function SessionsPage() {
	const navigate = useNavigate();
	const [sessions, setSessions] = useState<Session[]>([]);
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
	const filterInputRef = useRef<HTMLInputElement>(null);

	useEffect(() => {
		let cancelled = false;
		async function load() {
			try {
				const data = (await wfetch("/sessions")) as Session[];
				if (!cancelled) setSessions(data);
			} catch (err) {
				console.error("Failed to load sessions", err);
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

	const filtered = sessions.filter((s) => {
		const haystack = `${s.title || ""} ${s.id}`.toLowerCase();
		return haystack.includes(filter.toLowerCase());
	});

	async function handleCreate() {
		setCreating(true);
		try {
			const session = (await wfetch("/sessions", {
				method: "POST",
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

	async function chooseWorkingDirectory() {
		const picker = (window as Window & {
			showDirectoryPicker?: () => Promise<{ name: string }>;
		}).showDirectoryPicker;
		if (!picker) {
			alert("This browser does not support directory picking. Enter the path manually.");
			return;
		}
		try {
			const handle = await picker.call(window);
			setEditWorkDir(handle.name);
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
			setSessions((prev) => prev.map((session) => session.id === updated.id ? updated : session));
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
			setSessions((prev) => prev.filter((s) => s.id !== session.id));
			setDeleteSession(null);
		} catch (err) {
			alert(String(err));
		} finally {
			setDeletingSessionId("");
		}
	}

	return (
		<div className="mx-auto max-w-5xl px-4 py-6">
			<div className="mb-4">
				<PageBreadcrumb items={[{ label: "Sessions" }]} />
				<div className="mt-4 flex items-center justify-between gap-3">
					<Button size="sm" onClick={handleCreate} disabled={creating}>
						<PlusIcon className="size-4" />
						{creating ? "Creating..." : "New"}
					</Button>

					<div
						className={`flex h-9 items-center rounded-md border bg-card text-muted-foreground shadow-sm transition-all duration-200 focus-within:text-foreground hover:bg-accent hover:text-foreground ${filterOpen || filter ? "w-64 gap-2 px-2" : "w-9 justify-center"
							}`}
					>
						<button
							type="button"
							className="grid size-4 shrink-0 place-items-center"
							onClick={() => setFilterOpen(true)}
							aria-label="Filter sessions"
						>
							<MagnifyingGlassIcon className="size-4" />
						</button>
						<input
							ref={filterInputRef}
							placeholder="Filter sessions..."
							value={filter}
							onChange={(e) => setFilter(e.target.value)}
							tabIndex={filterOpen || filter ? 0 : -1}
							className={`h-7 min-w-0 border-0 bg-transparent p-0 text-sm text-inherit outline-none placeholder:text-muted-foreground ${filterOpen || filter ? "w-full opacity-100" : "w-0 opacity-0"
								}`}
						/>
						{(filterOpen || filter) && (
							<button
								type="button"
								className="grid size-4 shrink-0 place-items-center rounded-sm text-muted-foreground transition-colors hover:text-foreground"
								onClick={() => {
									setFilter("");
									setFilterOpen(false);
								}}
								aria-label="Close filter"
							>
								<XIcon className="size-3" />
							</button>
						)}
					</div>
				</div>
			</div>

			{loading ? (
				<div className="py-8 text-sm text-muted-foreground">Loading...</div>
			) : filtered.length === 0 && filter ? (
				<Empty>
					<EmptyTitle>No sessions found</EmptyTitle>
					<EmptyDescription>Try a different search.</EmptyDescription>
				</Empty>
			) : filtered.length === 0 ? (
				<Empty>
					<EmptyTitle>No sessions yet</EmptyTitle>
					<EmptyDescription>Start a new session to begin chatting.</EmptyDescription>
					<EmptyActions>
						<Button size="sm" onClick={handleCreate} disabled={creating}>
							<PlusIcon className="size-4" />
							{creating ? "Creating..." : "New"}
						</Button>
					</EmptyActions>
				</Empty>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Title</TableHead>
							<TableHead>Created</TableHead>
							<TableHead>Workdir</TableHead>
							<TableHead className="w-0">
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{filtered.map((s) => (
							<ContextMenu key={s.id}>
								<ContextMenuTrigger
									render={
										<TableRow
											className="cursor-pointer"
											onClick={() =>
												navigate({
													to: "/sessions/$sessionId",
													params: { sessionId: s.id },
												})
											}
										/>
									}
								>
									<TableCell className="font-medium">
										{s.title || s.id}
									</TableCell>
									<TableCell className="text-muted-foreground">
										{timeAgo(s.created_at)}
									</TableCell>
									<TableCell className="max-w-[200px] truncate text-muted-foreground">
										{s.work_dir || "—"}
									</TableCell>
									<TableCell className="w-0 text-right">
										<DropdownMenu>
											<DropdownMenuTrigger
												render={
													<Button
														variant="ghost"
														size="icon-sm"
														onClick={(e) => e.stopPropagation()}
														aria-label="Session actions"
													/>
												}
											>
												<DotsThreeVerticalIcon className="size-4" />
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end" className="w-44">
												<DropdownMenuItem onClick={() => openEdit(s)}>
													<PencilSimpleIcon className="size-4" />
													Edit session
												</DropdownMenuItem>
												<DropdownMenuSeparator />
												<DropdownMenuItem
													variant="destructive"
													disabled={deletingSessionId === s.id}
													onClick={() => setDeleteSession(s)}
												>
													<TrashIcon className="size-4" />
													{deletingSessionId === s.id ? "Deleting..." : "Delete session"}
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</ContextMenuTrigger>
								<ContextMenuContent className="w-44">
									<ContextMenuItem onClick={() => openEdit(s)}>
										<PencilSimpleIcon className="size-4" />
										Edit session
									</ContextMenuItem>
									<ContextMenuSeparator />
									<ContextMenuItem
										variant="destructive"
										disabled={deletingSessionId === s.id}
										onClick={() => setDeleteSession(s)}
									>
										<TrashIcon className="size-4" />
										{deletingSessionId === s.id ? "Deleting..." : "Delete session"}
									</ContextMenuItem>
								</ContextMenuContent>
							</ContextMenu>
						))}
					</TableBody>
				</Table>
			)}

			<Dialog open={editingSession !== null} onOpenChange={(open) => !open && setEditingSession(null)}>
				<DialogContent>
					<form onSubmit={handleEdit} className="grid gap-4">
						<DialogHeader>
							<DialogTitle>Edit session</DialogTitle>
							<DialogDescription>
								Change the session name or working directory.
							</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<div className="grid gap-1">
								<label className="text-xs font-medium">Name</label>
								<Input
									placeholder="Session name"
									value={editTitle}
									onChange={(e) => setEditTitle(e.target.value)}
								/>
							</div>
							<div className="grid gap-1">
								<label className="text-xs font-medium">Working directory</label>
								<div className="flex gap-2">
									<Input
										placeholder="Optional working directory"
										value={editWorkDir}
										onChange={(e) => setEditWorkDir(e.target.value)}
									/>
									<Button type="button" variant="outline" onClick={chooseWorkingDirectory}>
										<FolderOpenIcon className="size-4" />
										Choose
									</Button>
								</div>
								<p className="text-xs text-muted-foreground">
									Browsers do not expose absolute folder paths; enter the server path manually if needed.
								</p>
							</div>
						</div>
						<DialogFooter>
							<Button type="button" variant="outline" onClick={() => setEditingSession(null)} disabled={savingEdit}>
								Cancel
							</Button>
							<Button type="submit" disabled={savingEdit}>
								{savingEdit ? "Saving..." : "Save changes"}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={deleteSession !== null} onOpenChange={(open) => !open && setDeleteSession(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete session?</AlertDialogTitle>
						<AlertDialogDescription>
							This will permanently delete {deleteSession?.title || deleteSession?.id}. This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={!!deletingSessionId}>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							disabled={!deleteSession || !!deletingSessionId}
							onClick={() => deleteSession && handleDelete(deleteSession)}
						>
							{deletingSessionId ? "Deleting..." : "Delete"}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
