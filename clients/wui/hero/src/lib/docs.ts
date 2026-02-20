import { docs, type Doc } from "virtual:docs";

export type { Doc };

export interface DocGroup {
  name: string;
  docs: Doc[];
}

export function getGroupedDocs(): DocGroup[] {
  const groupMap = new Map<string, Doc[]>();
  for (const doc of docs) {
    const existing = groupMap.get(doc.group) || [];
    existing.push(doc);
    groupMap.set(doc.group, existing);
  }
  const groups: DocGroup[] = [];
  for (const [name, groupDocs] of groupMap) {
    groups.push({
      name,
      docs: groupDocs.sort((a, b) => a.order - b.order),
    });
  }
  return groups.sort((a, b) => {
    const aMin = Math.min(...a.docs.map((d) => d.order));
    const bMin = Math.min(...b.docs.map((d) => d.order));
    return aMin - bMin;
  });
}

export function getDocBySlug(slug: string): Doc | undefined {
  return docs.find((d) => d.slug === slug);
}

export function getFirstDoc(): Doc | undefined {
  const groups = getGroupedDocs();
  return groups[0]?.docs[0];
}
