import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import WingmanIcon from "@/assets/icon-128.png";
import { Button } from "@/components/core/button";
import { FileTextIcon, GearIcon, LightningIcon, SolarRoofIcon, StackIcon } from "@phosphor-icons/react";
import { cn } from "@/lib/utils";

function NavLink({
	to,
	icon: Icon,
	label,
}: {
	to: string;
	icon: React.ComponentType<{ size?: number; className?: string }>;
	label: string;
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
				!isActive && "text-muted-foreground"
			)}
		>
			<Icon size={16} />
			{label}
		</Button>
	);
}

export default function App() {
	const { location } = useRouterState();
	const isSessionDetail = /^\/sessions\/[^/]+/.test(location.pathname);

	return (
		<div className="flex min-h-dvh flex-col">
			<header className={cn("flex items-center justify-between gap-4 border-b px-4 py-3", isSessionDetail && "hidden")}>
				<div className="flex items-center gap-5">
					<Link to="/">
						<img src={WingmanIcon} className="w-8 h-8" alt="Wingman logo" />
					</Link>
					<nav className="flex items-center gap-3 text-xs text-muted-foreground">
						<NavLink to="/sessions" icon={StackIcon} label="Sessions" />
						<NavLink to="/agents" icon={LightningIcon} label="Agents" />
						<NavLink to="/providers" icon={SolarRoofIcon} label="Providers" />
						<NavLink to="/logs" icon={FileTextIcon} label="Logs" />
						<NavLink to="/settings" icon={GearIcon} label="Settings" />
					</nav>
				</div>
			</header>
			<main className="flex-1">
				<Outlet />
			</main>
		</div>
	);
}
