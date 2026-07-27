import { type ComponentProps, useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import { getSingletonHighlighter } from "shiki";
import { useTheme } from "@wingman/core/components/theme-provider";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table";

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const { theme, resolvedColorMode } = useTheme();
  const [html, setHtml] = useState("");

  useEffect(() => {
    let mounted = true;
    async function run() {
      const highlighter = await getSingletonHighlighter({
        themes: ["github-dark", "github-light", "gruvbox-dark-medium", "gruvbox-light-medium", "dracula", "nord", "rose-pine", "rose-pine-dawn"],
        langs: [
          "javascript",
          "typescript",
          "go",
          "python",
          "bash",
          "json",
          "markdown",
          "text",
          "html",
          "css",
          "yaml",
          "rust",
          "java",
          "sql",
        ],
      });
      const syntaxTheme = theme.shiki[resolvedColorMode] ?? theme.shiki.dark ?? "github-dark";
      const result = highlighter.codeToHtml(code, {
        lang: lang || "text",
        theme: syntaxTheme,
      });
      if (mounted) setHtml(result);
    }
    run();
    return () => {
      mounted = false;
    };
  }, [code, lang, resolvedColorMode, theme]);

  if (!html) {
    return (
      <div className="my-3 overflow-hidden rounded-xl border bg-card shadow-sm shadow-primary/5">
        {lang && (
          <div className="border-b bg-muted/45 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            {lang}
          </div>
        )}
        <pre className="overflow-x-auto p-4 text-[0.82rem] leading-6">
          <code>{code}</code>
        </pre>
      </div>
    );
  }

  return (
    <div className="my-3 overflow-hidden rounded-xl border bg-card shadow-sm shadow-primary/5">
      {lang && (
        <div className="border-b bg-muted/45 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {lang}
        </div>
      )}
      <div
        className="[&_.shiki]:m-0 [&_.shiki]:overflow-x-auto [&_.shiki]:bg-transparent! [&_.shiki]:p-4 [&_.shiki]:text-[0.82rem] [&_.shiki]:leading-6 [&_.shiki_code]:font-mono"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}

function PlainCodeBlock({ code, lang }: { code: string; lang?: string }) {
  return (
    <div className="my-3 overflow-hidden rounded-xl border bg-card shadow-sm shadow-primary/5">
      {lang && (
        <div className="border-b bg-muted/45 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {lang}
        </div>
      )}
      <pre className="overflow-x-auto p-4 text-[0.82rem] leading-6">
        <code>{code}</code>
      </pre>
    </div>
  );
}

function MarkdownTable({ children }: ComponentProps<"table">) {
  return <Table>{children}</Table>;
}

function MarkdownTableHeader({ children }: ComponentProps<"thead">) {
  return <TableHeader>{children}</TableHeader>;
}

function MarkdownTableBody({ children }: ComponentProps<"tbody">) {
  return <TableBody>{children}</TableBody>;
}

function MarkdownTableRow({ children }: ComponentProps<"tr">) {
  return <TableRow>{children}</TableRow>;
}

function MarkdownTableHead({ children }: ComponentProps<"th">) {
  return <TableHead>{children}</TableHead>;
}

function MarkdownTableCell({ children }: ComponentProps<"td">) {
  return <TableCell>{children}</TableCell>;
}

export function Markdown({ text, isStreaming = false }: { text: string; isStreaming?: boolean }) {
  return (
    <div className="space-y-2 text-sm leading-relaxed [&_p]:my-1.5 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-0.5 [&_li::marker]:text-[var(--markdown-list-marker)] [&_h1]:text-lg [&_h1]:font-semibold [&_h1]:text-[var(--markdown-heading)] [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-[var(--markdown-heading)] [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:text-[var(--markdown-heading)] [&_strong]:font-semibold [&_strong]:text-[var(--markdown-strong)] [&_b]:font-semibold [&_b]:text-[var(--markdown-strong)] [&_em]:text-[var(--markdown-emph)] [&_i]:text-[var(--markdown-emph)] [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-[color:var(--markdown-quote)] [&_blockquote]:pl-3 [&_blockquote]:text-[var(--markdown-quote)] [&_blockquote]:italic [&_hr]:my-3">
      <ReactMarkdown
        remarkPlugins={isStreaming ? [remarkGfm] : [remarkGfm, remarkMath]}
        rehypePlugins={isStreaming ? [] : [rehypeKatex]}
        components={{
          pre({ children }) {
            return <>{children}</>;
          },
          code({ className, children }) {
            const match = /language-(\w+)/.exec(className || "");
            const code = String(children).replace(/\n$/, "");
            if (!match) {
              return (
                <code className="rounded-md border bg-muted/55 px-1.5 py-0.5 text-[0.82em] font-medium text-[var(--markdown-code)]">
                  {children}
                </code>
              );
            }
            if (isStreaming) {
              return <PlainCodeBlock code={code} lang={match[1]} />;
            }
            return <CodeBlock code={code} lang={match[1]} />;
          },
          table: MarkdownTable,
          thead: MarkdownTableHeader,
          tbody: MarkdownTableBody,
          tr: MarkdownTableRow,
          th: MarkdownTableHead,
          td: MarkdownTableCell,
        }}
      >
        {text}
      </ReactMarkdown>
      {isStreaming && <span aria-hidden="true" className="streaming-caret" />}
    </div>
  );
}
