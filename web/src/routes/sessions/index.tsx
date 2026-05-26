import { useEffect, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { CaretRightIcon, FolderOpenIcon, PencilSimpleIcon, PlusIcon } from "@phosphor-icons/react";

import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/core/dialog";
import { baseSlug } from "@/lib/base-slug";
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
	const [loading, setLoading] = useState(true);
	const [creating, setCreating] = useState(false);
	const [baseDialogOpen, setBaseDialogOpen] = useState(false);
	const [editingBase, setEditingBase] = useState<Base | null>(null);
	const [baseName, setBaseName] = useState("");
	const [basePath, setBasePath] = useState("");
	const [savingBase, setSavingBase] = useState(false);

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
	}

	useEffect(() => {
		let cancelled = false;
		async function load() {
			try {
				await loadBases();
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

	async function handleCreate(base: Base) {
		setCreating(true);
		try {
			const session = (await wfetch("/sessions", {
				method: "POST",
				body: JSON.stringify({ base_id: base.id }),
			})) as Session;
			navigate({
				to: "/sessions/$baseSlug/$sessionId",
				params: { baseSlug: baseSlug(base), sessionId: session.id },
			});
		} catch (err) {
			alert(String(err));
		} finally {
			setCreating(false);
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
				await wfetch(`/bases/${editingBase.id}`, {
					method: "PUT",
					body: JSON.stringify(payload),
				});
			} else {
				const created = (await wfetch("/bases", {
					method: "POST",
					body: JSON.stringify(payload),
				})) as Base;
				navigate({ to: "/sessions/$baseSlug", params: { baseSlug: baseSlug(created) } });
			}
			await loadBases();
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
				<div>
					<div className="mb-5 flex items-center justify-between gap-3">
						<div className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
							Bases
						</div>
					</div>
					<div className="grid grid-cols-[repeat(auto-fill,14rem)] gap-4">
						{bases.map((base, index) => {
							const sessions = sessionsByBase[base.id] ?? [];
							const latest = latestSession(sessions);
							return (
								<div
									key={base.id}
									className={`group relative h-56 w-56 cursor-pointer rounded-xl border border-border/80 border-l-4 ${baseColors[index % baseColors.length]} bg-card p-5 shadow-sm transition-colors hover:bg-accent/35`}
									onClick={() => navigate({ to: "/sessions/$baseSlug", params: { baseSlug: baseSlug(base) } })}
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
										<Button size="icon-sm" variant="outline" disabled={creating} onClick={(e) => { e.stopPropagation(); handleCreate(base); }} aria-label="New session">
											<PlusIcon className="size-4" />
										</Button>
									</div>
								</div>
							);
						})}
						<button
							type="button"
							className="flex h-56 w-56 items-center justify-center rounded-xl border border-dashed border-border bg-background/40 font-mono text-sm text-muted-foreground transition-colors hover:bg-accent/30 hover:text-foreground"
							onClick={openCreateBase}
						>
							<PlusIcon className="mr-2 size-4" /> New base
						</button>
					</div>
				</div>
			)}

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
		</div>
	);
}
