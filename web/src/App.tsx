import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import WingmanIcon from "@/assets/icon-128.png";
import { Gear, Lightning, SolarRoof, Stack } from "@phosphor-icons/react";
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
		<Link
			to={to}
			className={cn(
				"flex items-center gap-2 rounded-md border shadow-sm p-2 text-xs transition-colors",
				isActive
					? "bg-primary text-primary-foreground border-primary"
					: "bg-card text-muted-foreground hover:text-foreground hover:bg-accent"
			)}
		>
			<Icon size={16} />
			{label}
		</Link>
	);
}

export default function App() {
	return (
		<div className="flex min-h-screen flex-col">
			<header className="flex items-center justify-between gap-4 border-b px-4 py-3">
				<div className="flex items-center gap-5">
					<Link to="/">
						<img src={WingmanIcon} className="w-8 h-8" alt="Wingman logo" />
					</Link>
					<nav className="flex items-center gap-3 text-xs text-muted-foreground">
						<NavLink to="/sessions" icon={Stack} label="Sessions" />
						<NavLink to="/agents" icon={Lightning} label="Agents" />
						<NavLink to="/providers" icon={SolarRoof} label="Providers" />
						<NavLink to="/settings" icon={Gear} label="Settings" />
					</nav>
				</div>
			</header>
			<main className="flex-1">
				<Outlet />
			</main>
		</div>
	);
}
