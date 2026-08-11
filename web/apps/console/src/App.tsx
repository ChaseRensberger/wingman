import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import WingmanIcon from "@/assets/icon-128.png";
import { Button } from "@wingman/core/components/core/button";
import { Badge } from "@wingman/core/components/core/badge";
import { Input } from "@wingman/core/components/core/input";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@wingman/core/components/core/sheet";
import { ListIcon } from "@phosphor-icons/react";
import { CommandPalette } from "@/components/command-palette";
import { DaemonConnectionBanner, useDaemonConnection } from "@/components/daemon-connection";
import { navItems } from "@/lib/navigation";
import { client as wingman } from "@/lib/client";
import { cn } from "@/lib/utils";

type Client = { id: string; name: string };

function useCurrentClient(hasConnected: boolean, revision: number) {
	const [client, setClient] = useState<Client>();

	useEffect(() => {
		if (!hasConnected) return;
		let cancelled = false;
		void wingman.current.client()
			.then((current) => { if (!cancelled) setClient(current); })
			.catch(() => {});
		return () => { cancelled = true; };
	}, [hasConnected, revision]);

	return client;
}

function CurrentClientBadge({ client, variant }: { client?: Client; variant: "ghost" | "secondary" }) {
	const label = client?.name ?? client?.id ?? "Client";
	return <Badge className="h-8 px-2.5 sm:h-9" variant={variant} title={client ? `Current Wingman client: ${client.id}` : "Current Wingman client"}>Client: {label}</Badge>;
}

function LoginForm() {
	const [password, setPassword] = useState("");
	const [error, setError] = useState("");
	const [busy, setBusy] = useState(false);

	async function login(event: React.FormEvent) {
		event.preventDefault();
		setBusy(true);
		setError("");
		try {
			const response = await fetch("/auth/login", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) });
			if (!response.ok) throw new Error("Invalid daemon password.");
			window.location.reload();
		} catch (err) {
			setError(err instanceof Error ? err.message : "Unable to sign in.");
		} finally {
			setBusy(false);
		}
	}

	return <main className="grid min-h-dvh place-items-center p-4"><form className="w-full max-w-sm space-y-4 rounded-lg border bg-card p-6 shadow-sm" onSubmit={login}><div><h1 className="text-lg font-semibold">Wingman access</h1><p className="mt-1 text-sm text-muted-foreground">Enter the daemon password to open this Console.</p></div><Input type="password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Password" /><Button className="w-full" disabled={busy || !password}>{busy ? "Signing in..." : "Sign in"}</Button>{error ? <p className="text-sm text-destructive">{error}</p> : null}</form></main>;
}

function NavLink({
	to,
	icon: Icon,
	label,
	className,
	onNavigate,
}: {
	to: string;
	icon: React.ComponentType<{ size?: number; className?: string }>;
	label: string;
	className?: string;
	onNavigate?: () => void;
}) {
	const { location } = useRouterState();
	const isActive =
		location.pathname === to || location.pathname.startsWith(to + "/");

	return (
		<Button
			render={<Link to={to} />}
			nativeButton={false}
			variant={isActive ? "default" : "outline"}
			size="lg"
			className={cn(
				"gap-2 text-xs",
				!isActive && "text-muted-foreground",
				className,
			)}
			onClick={onNavigate}
		>
			<Icon size={16} />
			{label}
		</Button>
	);
}

export default function App() {
	const { location } = useRouterState();
	const [navigationOpen, setNavigationOpen] = useState(false);
	const { revision, hasConnected, failure } = useDaemonConnection();
	const client = useCurrentClient(hasConnected, revision);
	const isSessionDetail = /^\/sessions\/[^/]+$/.test(location.pathname);
	const activeNavItem = navItems.find(
		({ to }) => location.pathname === to || location.pathname.startsWith(to + "/"),
	);

	if (!hasConnected && failure?.includes("not authorized")) return <LoginForm />;

	return (
		<div className={cn("flex flex-col", isSessionDetail ? "h-dvh" : "min-h-screen")}>
			<CommandPalette />
			<DaemonConnectionBanner />
			{!isSessionDetail && (
				<>
					<header className="flex items-center justify-between gap-4 border-b px-4 py-3 sm:hidden">
						<Link to="/" className="flex items-center gap-3">
							<img src={WingmanIcon} className="size-8" alt="Wingman logo" />
							<span className="text-sm font-medium">{activeNavItem?.label ?? "Wingman"}</span>
						</Link>
						<div className="flex items-center gap-2"><CurrentClientBadge client={client} variant="secondary" /><Sheet open={navigationOpen} onOpenChange={setNavigationOpen}>
							<SheetTrigger
								render={<Button variant="outline" size="icon" aria-label="Open navigation"><ListIcon /></Button>}
							/>
							<SheetContent className="w-full max-w-sm gap-6">
								<SheetHeader>
									<SheetTitle>Navigation</SheetTitle>
								</SheetHeader>
								<nav className="flex flex-col gap-2">
									{navItems.map((item) => (
										<NavLink key={item.to} {...item} className="w-full justify-start text-sm" onNavigate={() => setNavigationOpen(false)} />
									))}
								</nav>
							</SheetContent>
						</Sheet></div>
					</header>
					<header className="hidden items-center justify-between gap-4 border-b px-4 py-3 sm:flex">
						<div className="flex items-center gap-5">
							<Link to="/">
								<img src={WingmanIcon} className="size-8" alt="Wingman logo" />
							</Link>
							<nav className="flex items-center gap-3 text-xs text-muted-foreground">
								{navItems.map((item) => <NavLink key={item.to} {...item} />)}
							</nav>
						</div>
						<CurrentClientBadge client={client} variant="ghost" />
					</header>
				</>
			)}
			<main className="flex-1 min-h-0">
				{hasConnected && <Outlet key={revision} />}
			</main>
		</div>
	);
}
