import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Session } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, ChevronRight, MessageSquare } from "lucide-react";
import { CreateSessionDialog } from "@/components/CreateSessionDialog";
import { Separator } from "@wingman/core/components/primitives/separator";

export const Route = createFileRoute("/sessions")({
  component: SessionsPage,
});

function SessionsPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);

  const queryClient = useQueryClient();

  const sessionsQuery = useQuery({
    queryKey: ["sessions"],
    queryFn: () => api.listSessions(),
  });

  const deleteSessionMutation = useMutation({
    mutationFn: (id: string) => api.deleteSession(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });

  const handleDelete = (id: string) => {
    deleteSessionMutation.mutate(id, {
      onSuccess: () => {
        if (selectedSession?.id === id) setSelectedSession(null);
      },
    });
  };

  const sessions = sessionsQuery.data ?? [];
  const errorMessage =
    (sessionsQuery.error instanceof Error && sessionsQuery.error.message) ||
    (deleteSessionMutation.error instanceof Error && deleteSessionMutation.error.message) ||
    null;

	return (
		<>
			<div className="flex items-center justify-between px-8 py-3.5">
				<h1 className="text-2xl font-semibold tracking-tight">Sessions</h1>
				<Dialog open={createOpen} onOpenChange={setCreateOpen}>
					<DialogTrigger asChild>
						<Button>
							<Plus className="size-6" />
							<span className="text-lg">New Session</span>
						</Button>
					</DialogTrigger>
					<CreateSessionDialog
						onCreated={() => {
							setCreateOpen(false);
						}}
					/>
				</Dialog>
			</div>
			<Separator className="m-0" />
			<div className="px-8 py-4">
				{errorMessage && (
					<div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
						{errorMessage}
					</div>
				)}

				{sessionsQuery.isLoading ? (
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
												disabled={deleteSessionMutation.isPending}
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
		</>
	);
}
