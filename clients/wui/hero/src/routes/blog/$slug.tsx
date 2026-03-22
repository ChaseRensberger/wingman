import { createFileRoute, Link } from "@tanstack/react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { getPostBySlug } from "@/lib/blog";
import WingmanIcon from "../../assets/WingmanBlue.png";

export const Route = createFileRoute("/blog/$slug")({
	component: BlogPost,
});

function BlogPost() {
	const { slug } = Route.useParams();
	const post = getPostBySlug(slug);

	if (!post) {
		return (
			<main className="min-h-screen flex flex-col md:max-w-3xl lg:max-w-4xl mx-auto border">
				<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
					<Link to="/">
						<img src={WingmanIcon} className="w-12 h-12" />
					</Link>
				</nav>
				<section className="flex-1 p-12">
					<p className="text-muted-foreground">Post not found.</p>
				</section>
			</main>
		);
	}

	return (
		<main className="min-h-screen flex flex-col md:max-w-3xl lg:max-w-4xl mx-auto border">
			<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<Link to="/">
					<img src={WingmanIcon} className="w-12 h-12" />
				</Link>
				<Link
					to="/blog"
					className="text-sm text-muted-foreground hover:text-foreground transition-colors"
				>
					&larr; All posts
				</Link>
			</nav>
			<section className="flex-1 border-b p-12">
				<article className="prose dark:prose-invert max-w-none">
					<header className="mb-8 not-prose">
						<h1 className="text-3xl font-semibold tracking-tight">{post.title}</h1>
						{post.date && (
							<p className="text-muted-foreground text-sm mt-2">{post.date}</p>
						)}
					</header>
					<ReactMarkdown remarkPlugins={[remarkGfm]}>
						{post.content}
					</ReactMarkdown>
				</article>
			</section>
			<footer className="px-6 py-2 text-center">
				<p className="text-sm text-muted-foreground font-mono">Wingman</p>
			</footer>
		</main>
	);
}
