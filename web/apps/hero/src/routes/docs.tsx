import { createFileRoute, Outlet, Link, useParams } from "@tanstack/react-router";
import WingmanIcon from "../assets/WingmanBlue.png";
import { getGroupedDocs } from "@/lib/docs";

export const Route = createFileRoute("/docs")({
	component: DocsLayout,
});

function DocsLayout() {
	const params = useParams({ strict: false });
	const slug = (params as { slug?: string }).slug;
	const groups = getGroupedDocs();

	return (
		<div className="min-h-screen flex flex-col">
			{/* Header */}
			<div className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<Link to="/">
					<img src={WingmanIcon} className="w-12 h-12" />
				</Link>
			</div>
			{/* Sidebar */}
			<div className='flex-1 flex'>
				<nav className='p-4 border-r w-64 space-y-4'>
					{groups.map((group) => (
						<div key={group.name}>
							<h3 className="font-semibold text-sm text-muted-foreground mb-2">
								{group.name}
							</h3>
							<ul className="space-y-1">
								{group.docs.map((doc) => (
									<li key={doc.slug}>
										<Link
											to="/docs/$slug"
											params={{ slug: doc.slug }}
											className={`block px-2 py-1 rounded text-sm hover:bg-muted ${slug === doc.slug ? "bg-muted font-medium" : ""
												}`}
										>
											{doc.title}
										</Link>
									</li>
								))}
							</ul>
						</div>
					))}
				</nav>
				{/* Main Content */}
				<main className='flex-1 p-8'>
					<Outlet />
				</main>
			</div>
		</div>
	);
}
