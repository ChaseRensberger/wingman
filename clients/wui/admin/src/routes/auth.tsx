import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type ProvidersAuthResponse } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, KeyRound } from "lucide-react";
import { CreateAuthDialog } from "@/components/CreateAuthDialog";
import { Separator } from "@wingman/core/components/primitives/separator";

export const Route = createFileRoute("/auth")({
  component: AuthPage,
});

function AuthPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const queryClient = useQueryClient();

  const authQuery = useQuery<ProvidersAuthResponse>({
    queryKey: ["providers-auth"],
    queryFn: () => api.getProvidersAuth(),
  });

  const deleteAuthMutation = useMutation({
    mutationFn: (provider: string) => api.deleteProviderAuth(provider),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["providers-auth"] });
    },
  });

  const providers = authQuery.data?.providers ?? {};
  const errorMessage =
    (authQuery.error instanceof Error && authQuery.error.message) ||
    (deleteAuthMutation.error instanceof Error && deleteAuthMutation.error.message) ||
    null;

	return (
		<>
			<div className="flex items-center justify-between px-8 py-3.5">
				<h1 className="text-2xl font-semibold tracking-tight">Auth</h1>
				<Dialog open={createOpen} onOpenChange={setCreateOpen}>
					<DialogTrigger asChild>
						<Button>
							<Plus className="size-6" />
							<span className="text-lg">Add Provider</span>
						</Button>
					</DialogTrigger>
					<CreateAuthDialog
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

				{authQuery.isLoading ? (
					<div className="text-sm text-muted-foreground">Loading...</div>
				) : Object.keys(providers).length === 0 ? (
					<div className="flex flex-col items-center justify-center py-12 text-center">
						<KeyRound className="size-10 text-muted-foreground mb-3" />
						<p className="text-sm text-muted-foreground">No providers configured. Add one to get started.</p>
					</div>
				) : (
					<div className="grid gap-4 md:grid-cols-2">
						{Object.entries(providers).map(([name, info]) => (
							<Card key={name}>
								<CardHeader className="pb-2">
									<div className="flex items-center justify-between">
										<CardTitle className="text-sm font-medium capitalize">{name}</CardTitle>
										<div className="flex items-center gap-2">
											<Badge variant={info.configured ? "default" : "secondary"}>
												{info.configured ? "Configured" : "Not configured"}
											</Badge>
											<Button
												variant="ghost"
												size="icon"
												className="size-7"
												onClick={() => deleteAuthMutation.mutate(name)}
												disabled={deleteAuthMutation.isPending}
											>
												<Trash2 className="size-3.5 text-muted-foreground" />
											</Button>
										</div>
									</div>
								</CardHeader>
								<CardContent className="pt-0">
									<p className="text-xs text-muted-foreground">Type: {info.type}</p>
								</CardContent>
							</Card>
						))}
					</div>
				)}

				{authQuery.data?.updated_at && (
					<p className="text-xs text-muted-foreground">Last updated {new Date(authQuery.data.updated_at).toLocaleString()}</p>
				)}
			</div>
		</>
	);
}
