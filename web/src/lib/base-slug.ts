import type { Base } from "@/lib/types";

export function baseSlug(base: Base) {
	const slug = base.name
		.trim()
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
	return slug || base.id;
}

export function findBaseBySlug(bases: Base[], slug: string) {
	return bases.find((base) => baseSlug(base) === slug) ?? null;
}
