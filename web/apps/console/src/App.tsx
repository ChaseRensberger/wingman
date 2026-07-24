import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { useState } from "react";
import WingmanIcon from "@/assets/icon-128.png";
import { Button } from "@wingman/core/components/core/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@wingman/core/components/core/sheet";
import { FileTextIcon, GearIcon, LightningIcon, ListIcon, SolarRoofIcon, StackIcon, WrenchIcon } from "@phosphor-icons/react";
import { CommandPalette } from "@/components/command-palette";
import { cn } from "@/lib/utils";

const navItems = [
	{ to: "/sessions", icon: StackIcon, label: "Sessions" },
	{ to: "/agents", icon: LightningIcon, label: "Agents" },
	{ to: "/tools", icon: WrenchIcon, label: "Tools" },
	{ to: "/providers", icon: SolarRoofIcon, label: "Providers" },
	{ to: "/logs", icon: FileTextIcon, label: "Logs" },
	{ to: "/settings", icon: GearIcon, label: "Settings" },
];

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
	const isSessionDetail = /^\/sessions\/[^/]+$/.test(location.pathname);
	const activeNavItem = navItems.find(
		({ to }) => location.pathname === to || location.pathname.startsWith(to + "/"),
	);

	return (
		<div className={cn("flex flex-col", isSessionDetail ? "h-dvh" : "min-h-screen")}>
			<CommandPalette />
			{!isSessionDetail && (
				<>
					<header className="flex items-center justify-between gap-4 border-b px-4 py-3 sm:hidden">
						<Link to="/" className="flex items-center gap-3">
							<img src={WingmanIcon} className="size-8" alt="Wingman logo" />
							<span className="text-sm font-medium">{activeNavItem?.label ?? "Wingman"}</span>
						</Link>
						<Sheet open={navigationOpen} onOpenChange={setNavigationOpen}>
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
						</Sheet>
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
					</header>
				</>
			)}
			<main className="flex-1 min-h-0">
				<Outlet />
			</main>
		</div>
	);
}
