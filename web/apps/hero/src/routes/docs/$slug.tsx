import { createFileRoute } from "@tanstack/react-router";
import ReactMarkdown from "react-markdown";
import { getDocBySlug } from "@/lib/docs";

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
		<article className="prose prose-neutral dark:prose-invert">
			<ReactMarkdown>{doc.content}</ReactMarkdown>
		</article>
	);
}
