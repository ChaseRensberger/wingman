import { useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { wfetch } from "@/lib/client";
import type { Session } from "@/lib/types";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/core/dialog";
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
  EmptyIcon,
  EmptyTitle,
  EmptyDescription,
  EmptyActions,
} from "@/components/core/empty";
import { PlusIcon, ChatTeardropTextIcon } from "@phosphor-icons/react";

function timeAgo(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return date.toLocaleDateString();
}

export default function SessionsPage() {
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newWorkDir, setNewWorkDir] = useState("");
  const [creating, setCreating] = useState(false);

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
      setDialogOpen(false);
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
      <div className="mb-4 flex items-center justify-between">
        <div className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">Sessions</span>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger render={<Button size="sm"><PlusIcon className="size-4" />New session</Button>} />
          <DialogContent>
            <form onSubmit={handleCreate}>
              <DialogHeader>
                <DialogTitle>New session</DialogTitle>
                <DialogDescription>
                  Create a new conversation session.
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-3 py-4">
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
              </div>
              <DialogFooter>
                <Button type="submit" disabled={creating}>
                  {creating ? "Creating..." : "Create"}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Input
        placeholder="Filter sessions..."
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        className="mb-4"
      />

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : filtered.length === 0 ? (
        <Empty>
          <EmptyIcon>
            <ChatTeardropTextIcon className="size-7" />
          </EmptyIcon>
          <EmptyTitle>No sessions yet</EmptyTitle>
          <EmptyDescription>Start a new session to begin chatting.</EmptyDescription>
          <EmptyActions>
            <Button size="sm" onClick={() => setDialogOpen(true)}>
              <PlusIcon className="size-4" />
              New session
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
