import { createFileRoute } from "@tanstack/react-router";
import ReactMarkdown from "react-markdown";
import { getDocBySlug } from "@/lib/docs";
import remarkGfm from "remark-gfm";

export const Route = createFileRoute("/docs/$slug")({
	component: DocPage,
});

function DocPage() {
	const { slug } = Route.useParams();
	const doc = getDocBySlug(slug);

	if (!doc) {
		return <p>Doc not found.</p>;
	}

	return (
		<article className="prose prose-invert max-w-4xl">
			<ReactMarkdown remarkPlugins={[remarkGfm]}>{doc.content}</ReactMarkdown>
		</article>
	);
}
