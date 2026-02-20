import { createFileRoute, Navigate } from "@tanstack/react-router";
import { getFirstDoc } from "@/lib/docs";

export const Route = createFileRoute("/docs/")({
	component: DocsIndex,
});

function DocsIndex() {
	const firstDoc = getFirstDoc();
	if (firstDoc) {
		return <Navigate to="/docs/$slug" params={{ slug: firstDoc.slug }} />;
	}
	return <p>No docs found.</p>;
}
