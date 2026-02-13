import { createFileRoute, Outlet, Link } from "@tanstack/react-router";
import WingmanIcon from "../assets/WingmanBlue.png";

export const Route = createFileRoute("/blog")({
	component: BlogLayout,
});

function BlogLayout() {
	return (
		<div className="min-h-screen flex flex-col w-full max-w-full">
			<div className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b z-20">
				<Link to="/">
					<img src={WingmanIcon} className="w-12 h-12" />
				</Link>
			</div>
			<main className="flex-1 w-full max-w-3xl mx-auto p-8">
				<Outlet />
			</main>
		</div>
	);
}
