import { createFileRoute, Link } from "@tanstack/react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { getPostBySlug } from "@/lib/blog";

export const Route = createFileRoute("/blog/$slug")({
	component: BlogPost,
});

function BlogPost() {
	const { slug } = Route.useParams();
	const post = getPostBySlug(slug);

	if (!post) {
		return <p>Post not found.</p>;
	}

	return (
		<div className="space-y-6">
			<Link
				to="/blog"
				className="text-sm text-muted-foreground hover:text-foreground transition-colors"
			>
				&larr; Back to blog
			</Link>
			<article className="prose dark:prose-invert max-w-none">
				<h1>{post.title}</h1>
				{post.date && (
					<p className="text-muted-foreground text-sm !mt-0">{post.date}</p>
				)}
				<ReactMarkdown remarkPlugins={[remarkGfm]}>{post.content}</ReactMarkdown>
			</article>
		</div>
	);
}
