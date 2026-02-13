import { createFileRoute, Link } from "@tanstack/react-router";
import { getAllPosts } from "@/lib/blog";
import WingmanIcon from "../../assets/WingmanBlue.png";

export const Route = createFileRoute("/blog/")({
	component: BlogIndex,
});

function BlogIndex() {
	const posts = getAllPosts();

	if (posts.length === 0) {
		return <p className="text-muted-foreground">No posts yet.</p>;
	}

	return (

		<main className="min-h-screen flex flex-col md:max-w-3xl lg:max-w-4xl mx-auto border">

			<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<Link to="/">
					<img src={WingmanIcon} className="w-12 h-12" />
				</Link>
				<div className="flex items-center gap-6">
				</div>
			</nav>
			<section className="flex-1 border-b p-12 space-y-8">
				<h1 className="text-4xl text-primary font-semibold text-center tracking-widest">BLOG</h1>
				<div className="space-y-4">
					{posts.map((post) => (
						<Link
							key={post.slug}
							to="/blog/$slug"
							params={{ slug: post.slug }}
							className="block group"
						>
							<article className="flex items-center justify-between">
								<h2 className="text-lg font-medium group-hover:underline">
									{post.title}
								</h2>
								{post.date && (
									<p className="text-muted-foreground">{post.date}</p>
								)}
							</article>
						</Link>
					))}
				</div>
			</section>
			<footer className="px-6 py-2 text-center">
				<p className="text-sm text-muted-foreground font-mono">
					Hero
				</p>
			</footer>
		</main>
	);
}
