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
import { MagnifyingGlassIcon, PlusIcon, XIcon } from "@phosphor-icons/react";

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
  const [createOpen, setCreateOpen] = useState(false);
  const [filterOpen, setFilterOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newWorkDir, setNewWorkDir] = useState("");
  const [creating, setCreating] = useState(false);
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

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    try {
      const body: Record<string, string> = {};
      if (newTitle.trim()) body.title = newTitle.trim();
      if (newWorkDir.trim()) body.working_directory = newWorkDir.trim();
      const session = (await wfetch("/sessions", {
        method: "POST",
        body: JSON.stringify(body),
      })) as Session;
      setCreateOpen(false);
      setNewTitle("");
      setNewWorkDir("");
      navigate({ to: "/sessions/$sessionId", params: { sessionId: session.id } });
    } catch (err) {
      alert(String(err));
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Sessions" }]} />
        <div className="mt-4 flex items-center justify-between gap-3">
          <Button size="sm" onClick={() => setCreateOpen((open) => !open)}>
            <PlusIcon className="size-4" />
            New
          </Button>

          <div
            className={`flex h-9 items-center rounded-md border bg-card text-muted-foreground shadow-sm transition-all duration-200 focus-within:text-foreground hover:bg-accent hover:text-foreground ${
              filterOpen || filter ? "w-64 gap-2 px-2" : "w-9 justify-center"
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
                className={`h-7 min-w-0 border-0 bg-transparent p-0 text-sm text-inherit outline-none placeholder:text-muted-foreground ${
                  filterOpen || filter ? "w-full opacity-100" : "w-0 opacity-0"
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

      {createOpen && (
        <form onSubmit={handleCreate} className="mb-4 rounded-xl border bg-card p-4 shadow-sm shadow-primary/5">
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <div className="grid gap-1">
              <label className="text-xs font-medium">Title</label>
              <Input
                placeholder="Optional title"
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
              />
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium">Working directory</label>
              <Input
                placeholder="Optional working directory"
                value={newWorkDir}
                onChange={(e) => setNewWorkDir(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={creating}>
              {creating ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      )}

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
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <PlusIcon className="size-4" />
              New
            </Button>
          </EmptyActions>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Agent</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Workdir</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((s) => (
              <TableRow
                key={s.id}
                className="cursor-pointer"
                onClick={() =>
                  navigate({
                    to: "/sessions/$sessionId",
                    params: { sessionId: s.id },
                  })
                }
              >
                <TableCell className="font-medium">
                  {s.title || s.id}
                </TableCell>
                <TableCell className="text-muted-foreground">—</TableCell>
                <TableCell className="text-muted-foreground">—</TableCell>
                <TableCell className="text-muted-foreground">
                  {timeAgo(s.created_at)}
                </TableCell>
                <TableCell className="max-w-[200px] truncate text-muted-foreground">
                  {s.work_dir || "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
