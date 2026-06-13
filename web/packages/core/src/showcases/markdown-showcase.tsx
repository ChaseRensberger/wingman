import { Markdown } from "#components/core/markdown"

const sampleMarkdown = `# Markdown

Render GitHub-flavored markdown with WingUI tokens, including **strong text**, _emphasis_, [links](https://github.com/wingman-actor), and \`inline code\`.

> Markdown is useful for chat transcripts, docs, agent output, and generated explanations.

## Lists

- Syntax highlighted code blocks
- Tables via GFM
- Inline code and blockquotes

## Table

| Language | Highlighted |
| --- | --- |
| TypeScript | yes |
| Go | yes |
| Markdown | yes |

## Code

\`\`\`tsx
type Status = "idle" | "loading" | "ready"

function StatusBadge({ status }: { status: Status }) {
  return <span data-status={status}>{status}</span>
}
\`\`\`
`

export function MarkdownShowcase() {
  return (
    <section className="py-4 space-y-4">
      <h2 className="text-2xl font-semibold">Markdown</h2>
      <div className="rounded-xl border bg-card p-6">
        <Markdown>{sampleMarkdown}</Markdown>
      </div>
    </section>
  )
}
