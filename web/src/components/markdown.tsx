import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { getSingletonHighlighter } from "shiki";
import { useTheme } from "@/components/theme-provider";

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const { theme } = useTheme();
  const [html, setHtml] = useState("");

  useEffect(() => {
    let mounted = true;
    async function run() {
      const highlighter = await getSingletonHighlighter({
        themes: ["github-dark", "github-light"],
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
      const isDark =
        theme === "dark" ||
        (theme === "system" &&
          window.matchMedia("(prefers-color-scheme: dark)").matches);
      const result = highlighter.codeToHtml(code, {
        lang: lang || "text",
        theme: isDark ? "github-dark" : "github-light",
      });
      if (mounted) setHtml(result);
    }
    run();
    return () => {
      mounted = false;
    };
  }, [code, lang, theme]);

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

export function Markdown({ text }: { text: string }) {
  return (
    <div className="space-y-2 text-sm leading-relaxed [&_p]:my-1.5 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-0.5 [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:text-sm [&_h3]:font-semibold [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-muted [&_blockquote]:pl-3 [&_blockquote]:italic [&_hr]:my-3 [&_table]:w-full [&_table]:text-left [&_td]:py-1 [&_th]:border-b [&_th]:py-1">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre({ children }) {
            return <>{children}</>;
          },
          code({ className, children }) {
            const match = /language-(\w+)/.exec(className || "");
            const code = String(children).replace(/\n$/, "");
            if (!match) {
              return (
                <code className="rounded-md border bg-muted/55 px-1.5 py-0.5 text-[0.82em] font-medium text-foreground">
                  {children}
                </code>
              );
            }
            return <CodeBlock code={code} lang={match[1]} />;
          },
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
