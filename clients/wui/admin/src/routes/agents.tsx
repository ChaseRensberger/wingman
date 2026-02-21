import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Agent } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, ChevronRight, Bot } from "lucide-react";
import { CreateAgentDialog } from "@/components/CreateAgentDialog";
import { Separator } from "@wingman/core/components/primitives/separator";

export const Route = createFileRoute("/agents")({
	component: AgentsPage,
});

function AgentsPage() {
	const [createOpen, setCreateOpen] = useState(false);
	const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null);

	const queryClient = useQueryClient();

	const agentsQuery = useQuery({
		queryKey: ["agents"],
		queryFn: () => api.listAgents(),
	});

	const deleteAgentMutation = useMutation({
		mutationFn: (id: string) => api.deleteAgent(id),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["agents"] });
		},
	});

	const handleDelete = (id: string) => {
		deleteAgentMutation.mutate(id, {
			onSuccess: () => {
				if (selectedAgent?.id === id) setSelectedAgent(null);
			},
		});
	};

	const agents = agentsQuery.data ?? [];
	const errorMessage =
		(agentsQuery.error instanceof Error && agentsQuery.error.message) ||
		(deleteAgentMutation.error instanceof Error && deleteAgentMutation.error.message) ||
		null;

	return (
		<>
			{/* Header */}
			<div className="flex items-center justify-between px-8 py-3.5">
				<h1 className="text-2xl font-semibold tracking-tight">Agents</h1>
				<Dialog open={createOpen} onOpenChange={setCreateOpen}>
					<DialogTrigger asChild>
						<Button>
							<Plus className="size-6" />
							<span className="text-lg">Create Agent</span>
						</Button>
					</DialogTrigger>
					<CreateAgentDialog
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

				{agentsQuery.isLoading ? (
					<div className="text-sm text-muted-foreground">Loading...</div>
				) : agents.length === 0 ? (
					<div className="flex flex-col items-center justify-center py-12 text-center">
						<Bot className="size-10 text-muted-foreground mb-3" />
						<p className="text-sm text-muted-foreground">No agents yet. Create one to get started.</p>
					</div>
				) : (
					<div className="grid gap-4">
						{agents.map((agent) => (
							<Card
								key={agent.id}
								className="cursor-pointer transition-colors hover:bg-accent/50"
								onClick={() => setSelectedAgent(selectedAgent?.id === agent.id ? null : agent)}
							>
								<CardHeader className="pb-2">
									<div className="flex items-center justify-between">
										<CardTitle className="text-sm font-medium">{agent.name}</CardTitle>
										<div className="flex items-center gap-1">
											<Button
												variant="ghost"
												size="icon"
												className="size-7"
												onClick={(e) => {
													e.stopPropagation();
													handleDelete(agent.id);
												}}
												disabled={deleteAgentMutation.isPending}
											>
												<Trash2 className="size-3.5 text-muted-foreground" />
											</Button>
											<ChevronRight
												className={`size-4 text-muted-foreground transition-transform ${selectedAgent?.id === agent.id ? "rotate-90" : ""}`}
											/>
										</div>
									</div>
									{agent.provider && (
										<CardDescription className="text-xs">
											{agent.provider.id}{agent.provider.model ? ` / ${agent.provider.model}` : ""}
										</CardDescription>
									)}
								</CardHeader>
								{selectedAgent?.id === agent.id && (
									<CardContent className="pt-0 space-y-3">
										{agent.instructions && (
											<div>
												<p className="text-xs font-medium text-muted-foreground mb-1">Instructions</p>
												<p className="text-xs whitespace-pre-wrap">{agent.instructions}</p>
											</div>
										)}
										{agent.tools && agent.tools.length > 0 && (
											<div>
												<p className="text-xs font-medium text-muted-foreground mb-1">Tools</p>
												<div className="flex flex-wrap gap-1">
													{agent.tools.map((tool) => (
														<Badge key={tool} variant="secondary" className="text-xs">
															{tool}
														</Badge>
													))}
												</div>
											</div>
										)}
										{agent.provider && (
											<div>
												<p className="text-xs font-medium text-muted-foreground mb-1">Provider</p>
												<div className="text-xs space-y-0.5">
													<p>ID: {agent.provider.id}</p>
													{agent.provider.model && <p>Model: {agent.provider.model}</p>}
													{agent.provider.max_tokens !== undefined && agent.provider.max_tokens > 0 && (
														<p>Max Tokens: {agent.provider.max_tokens}</p>
													)}
													{agent.provider.temperature !== undefined && agent.provider.temperature > 0 && (
														<p>Temperature: {agent.provider.temperature}</p>
													)}
												</div>
											</div>
										)}
										<p className="text-xs text-muted-foreground">
											Created {new Date(agent.created_at).toLocaleDateString()}
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
