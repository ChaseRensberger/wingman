import { Link, Outlet } from "@tanstack/react-router";
import { ThemeToggle } from "@/components/theme-toggle";

export default function App() {
	return (
		<div className="flex min-h-screen flex-col">
			<header className="flex items-center justify-between gap-4 border-b px-4 py-3">
				<div className="flex items-center gap-5">
					<Link to="/" className="text-sm font-semibold hover:underline">
						Wingman Web
					</Link>
					<nav className="flex items-center gap-3 text-xs text-muted-foreground">
						<Link to="/sessions" className="hover:text-foreground">
							Sessions
						</Link>
						<Link to="/agents" className="hover:text-foreground">
							Agents
						</Link>
						<Link to="/providers" className="hover:text-foreground">
							Providers
						</Link>
					</nav>
				</div>
				<ThemeToggle />
			</header>
			<main className="flex-1">
				<Outlet />
			</main>
		</div>
	);
}
