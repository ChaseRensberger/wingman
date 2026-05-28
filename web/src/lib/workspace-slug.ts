import type { Workspace } from "@/lib/types";

export function workspaceSlug(workspace: Workspace) {
	const slug = workspace.name
		.trim()
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
	return slug || workspace.id;
}

export function findWorkspaceBySlug(workspaces: Workspace[], slug: string) {
	return workspaces.find((workspace) => workspaceSlug(workspace) === slug) ?? null;
}
