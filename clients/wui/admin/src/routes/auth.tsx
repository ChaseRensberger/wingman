import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState, useCallback } from "react";
import { api, type ProvidersAuthResponse } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, KeyRound } from "lucide-react";
import { CreateAuthDialog } from "@/components/CreateAuthDialog";

export const Route = createFileRoute("/auth")({
	component: AuthPage,
});

function AuthPage() {
	const [auth, setAuth] = useState<ProvidersAuthResponse | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);
	const [createOpen, setCreateOpen] = useState(false);

	const fetchAuth = useCallback(async () => {
		try {
			setLoading(true);
			setError(null);
			const data = await api.getProvidersAuth();
			setAuth(data);
		} catch (e) {
			setError(e instanceof Error ? e.message : "Failed to fetch auth");
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		fetchAuth();
	}, [fetchAuth]);

	const handleDelete = async (provider: string) => {
		try {
			await api.deleteProviderAuth(provider);
			fetchAuth();
		} catch (e) {
			setError(e instanceof Error ? e.message : "Failed to delete auth");
		}
	};

	const providers = auth?.providers ?? {};

	return (
		<div className="space-y-4">
			<div className="flex items-center justify-between">
				<h1 className="text-2xl font-semibold tracking-tight">Auth</h1>
				<Dialog open={createOpen} onOpenChange={setCreateOpen}>
					<DialogTrigger asChild>
						<Button size="sm">
							<Plus className="size-4 mr-1" />
							Add Provider
						</Button>
					</DialogTrigger>
					<CreateAuthDialog
						onCreated={() => {
							setCreateOpen(false);
							fetchAuth();
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
											onClick={() => handleDelete(name)}
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

			{auth?.updated_at && (
				<p className="font-bold text-muted-foreground text-center">
					Last updated {new Date(auth.updated_at).toLocaleString()}
				</p>
			)}
		</div>
	);
}
