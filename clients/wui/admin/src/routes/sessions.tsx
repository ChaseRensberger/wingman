import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState, useCallback } from "react";
import { api, type Session } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, ChevronRight, MessageSquare } from "lucide-react";
import { CreateSessionDialog } from "@/components/CreateSessionDialog";

export const Route = createFileRoute("/sessions")({
  component: SessionsPage,
});

function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);

  const fetchSessions = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.listSessions();
      setSessions(data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to fetch sessions");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  const handleDelete = async (id: string) => {
    try {
      await api.deleteSession(id);
      if (selectedSession?.id === id) setSelectedSession(null);
      fetchSessions();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete session");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Sessions</h1>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button size="sm">
              <Plus className="size-4 mr-1" />
              New Session
            </Button>
          </DialogTrigger>
          <CreateSessionDialog
            onCreated={() => {
              setCreateOpen(false);
              fetchSessions();
            }}
          />
        </Dialog>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : sessions.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <MessageSquare className="size-10 text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">No sessions yet. Create one to get started.</p>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {sessions.map((session) => (
            <Card
              key={session.id}
              className="cursor-pointer transition-colors hover:bg-accent/50"
              onClick={() => setSelectedSession(selectedSession?.id === session.id ? null : session)}
            >
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-sm font-mono font-medium truncate max-w-[200px]">
                    {session.id}
                  </CardTitle>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDelete(session.id);
                      }}
                    >
                      <Trash2 className="size-3.5 text-muted-foreground" />
                    </Button>
                    <ChevronRight
                      className={`size-4 text-muted-foreground transition-transform ${selectedSession?.id === session.id ? "rotate-90" : ""}`}
                    />
                  </div>
                </div>
                <CardDescription className="text-xs">
                  Created {new Date(session.created_at).toLocaleDateString()}
                </CardDescription>
              </CardHeader>
              {selectedSession?.id === session.id && (
                <CardContent className="pt-0 space-y-2">
                  {session.work_dir && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-0.5">Work Dir</p>
                      <p className="text-xs font-mono">{session.work_dir}</p>
                    </div>
                  )}
                  <div>
                    <p className="text-xs font-medium text-muted-foreground mb-0.5">Messages</p>
                    <p className="text-xs">{session.history?.length ?? 0} messages</p>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Updated {new Date(session.updated_at).toLocaleDateString()}
                  </p>
                </CardContent>
              )}
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
