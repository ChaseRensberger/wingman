declare module "virtual:docs" {
  export interface Doc {
    slug: string;
    title: string;
    group: string;
    order: number;
    content: string;
  }
  export const docs: Doc[];
}
