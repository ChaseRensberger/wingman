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

declare module "virtual:blog" {
  export interface Post {
    slug: string;
    title: string;
    date: string;
    description: string;
    content: string;
  }
  export const posts: Post[];
}
