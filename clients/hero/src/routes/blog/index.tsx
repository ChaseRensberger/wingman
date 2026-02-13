import { createFileRoute, Link } from "@tanstack/react-router";
import { getAllPosts } from "@/lib/blog";

export const Route = createFileRoute("/blog/")({
	component: BlogIndex,
});

function BlogIndex() {
	const posts = getAllPosts();

	if (posts.length === 0) {
		return <p className="text-muted-foreground">No posts yet.</p>;
	}

	return (
		<div className="space-y-8">
			<h1 className="text-3xl font-semibold tracking-tight">Blog</h1>
			<div className="space-y-6">
				{posts.map((post) => (
					<Link
						key={post.slug}
						to="/blog/$slug"
						params={{ slug: post.slug }}
						className="block group"
					>
						<article className="space-y-1">
							<h2 className="text-lg font-medium group-hover:underline">
								{post.title}
							</h2>
							{post.description && (
								<p className="text-muted-foreground text-sm">
									{post.description}
								</p>
							)}
							{post.date && (
								<p className="text-muted-foreground text-xs">{post.date}</p>
							)}
						</article>
					</Link>
				))}
			</div>
		</div>
	);
}
