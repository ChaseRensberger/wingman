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
      <pre className="rounded border bg-muted p-3 text-sm">
        <code>{code}</code>
      </pre>
    );
  }

  return <div dangerouslySetInnerHTML={{ __html: html }} />;
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="space-y-2 text-sm leading-relaxed [&_p]:my-1.5 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-0.5 [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:text-sm [&_h3]:font-semibold [&_a]:underline [&_a]:text-primary [&_blockquote]:border-l-2 [&_blockquote]:border-muted [&_blockquote]:pl-3 [&_blockquote]:italic [&_hr]:my-3 [&_table]:w-full [&_table]:text-left [&_th]:border-b [&_th]:py-1 [&_td]:py-1">
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
              return <code className={className}>{children}</code>;
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
