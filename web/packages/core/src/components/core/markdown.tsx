import { useEffect, useState } from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { HighlighterCore } from "shiki/core"
import { useTheme } from "#components/theme-provider"
import { cn } from "#lib/utils"

const supportedLanguages = new Set([
  "bash",
  "css",
  "go",
  "html",
  "javascript",
  "json",
  "jsx",
  "markdown",
  "python",
  "rust",
  "sql",
  "text",
  "tsx",
  "typescript",
  "yaml",
])

const languageAliases: Record<string, string> = {
  js: "javascript",
  shell: "bash",
  shellscript: "bash",
  md: "markdown",
  py: "python",
  sh: "bash",
  ts: "typescript",
  yml: "yaml",
}

let highlighterPromise: Promise<HighlighterCore> | null = null

async function createMarkdownHighlighter() {
  const [
    { createHighlighterCore },
    { createJavaScriptRegexEngine },
    githubDark,
    githubLight,
    bash,
    css,
    go,
    html,
    javascript,
    json,
    jsx,
    markdown,
    python,
    rust,
    sql,
    tsx,
    typescript,
    yaml,
  ] = await Promise.all([
    import("shiki/core"),
    import("shiki/engine/javascript"),
    import("shiki/themes/github-dark.mjs"),
    import("shiki/themes/github-light.mjs"),
    import("shiki/langs/bash.mjs"),
    import("shiki/langs/css.mjs"),
    import("shiki/langs/go.mjs"),
    import("shiki/langs/html.mjs"),
    import("shiki/langs/javascript.mjs"),
    import("shiki/langs/json.mjs"),
    import("shiki/langs/jsx.mjs"),
    import("shiki/langs/markdown.mjs"),
    import("shiki/langs/python.mjs"),
    import("shiki/langs/rust.mjs"),
    import("shiki/langs/sql.mjs"),
    import("shiki/langs/tsx.mjs"),
    import("shiki/langs/typescript.mjs"),
    import("shiki/langs/yaml.mjs"),
  ])

  return createHighlighterCore({
    engine: createJavaScriptRegexEngine(),
    themes: [githubDark.default, githubLight.default],
    langs: [
      bash.default,
      css.default,
      go.default,
      html.default,
      javascript.default,
      json.default,
      jsx.default,
      markdown.default,
      python.default,
      rust.default,
      sql.default,
      tsx.default,
      typescript.default,
      yaml.default,
    ],
    langAlias: languageAliases,
  })
}

function getMarkdownHighlighter() {
  highlighterPromise ??= createMarkdownHighlighter()
  return highlighterPromise
}

function normalizeLanguage(lang?: string) {
  if (!lang) return "text"

  const normalized = languageAliases[lang] ?? lang
  return supportedLanguages.has(normalized) ? normalized : "text"
}

function useIsDarkTheme() {
  const { theme } = useTheme()

  if (theme === "dark") return true
  if (theme === "light") return false

  return window.matchMedia("(prefers-color-scheme: dark)").matches
}

function PlainCodeBlock({ code, lang }: { code: string; lang?: string }) {
  return (
    <div className="my-4 overflow-hidden rounded-xl border bg-card shadow-sm shadow-primary/5">
      {lang && (
        <div className="border-b bg-muted/45 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {lang}
        </div>
      )}
      <pre className="overflow-x-auto p-4 text-[0.82rem] leading-6">
        <code>{code}</code>
      </pre>
    </div>
  )
}

type HighlightedCode = {
  key: string
  html: string
}

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const isDark = useIsDarkTheme()
  const highlightKey = `${code}\u0000${lang}\u0000${isDark}`
  const [highlightedCode, setHighlightedCode] = useState<HighlightedCode | null>(null)

  useEffect(() => {
    let mounted = true
    const shikiLang = normalizeLanguage(lang)

    async function highlight() {
      const highlighter = await getMarkdownHighlighter()

      const highlighted = highlighter.codeToHtml(code, {
        lang: shikiLang,
        theme: isDark ? "github-dark" : "github-light",
      })

      if (mounted) setHighlightedCode({ key: highlightKey, html: highlighted })
    }

    highlight()

    return () => {
      mounted = false
    }
  }, [code, lang, isDark, highlightKey])

  if (highlightedCode?.key !== highlightKey) return <PlainCodeBlock code={code} lang={lang} />

  return (
    <div className="my-4 overflow-hidden rounded-xl border bg-card shadow-sm shadow-primary/5">
      {lang && (
        <div className="border-b bg-muted/45 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {lang}
        </div>
      )}
      <div
        className="[&_.shiki]:m-0 [&_.shiki]:overflow-x-auto [&_.shiki]:bg-transparent! [&_.shiki]:p-4 [&_.shiki]:text-[0.82rem] [&_.shiki]:leading-6 [&_.shiki_code]:font-mono"
        dangerouslySetInnerHTML={{ __html: highlightedCode.html }}
      />
    </div>
  )
}

type MarkdownProps = Omit<React.ComponentProps<"div">, "children"> & {
  children: string
}

function Markdown({ children, className, ...props }: MarkdownProps) {
  return (
    <div
      data-slot="markdown"
      className={cn(
        "space-y-3 text-sm leading-relaxed [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-muted [&_blockquote]:pl-3 [&_blockquote]:italic [&_h1]:scroll-m-20 [&_h1]:text-3xl [&_h1]:font-bold [&_h1]:tracking-tight [&_h2]:scroll-m-20 [&_h2]:text-2xl [&_h2]:font-semibold [&_h2]:tracking-tight [&_h3]:scroll-m-20 [&_h3]:text-xl [&_h3]:font-semibold [&_h4]:scroll-m-20 [&_h4]:text-lg [&_h4]:font-semibold [&_hr]:my-4 [&_hr]:border-border [&_li]:my-1 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:leading-7 [&_table]:w-full [&_table]:text-left [&_tbody_tr]:border-b [&_td]:py-2 [&_td]:pr-4 [&_th]:border-b [&_th]:py-2 [&_th]:pr-4 [&_th]:font-semibold [&_ul]:list-disc [&_ul]:pl-6",
        className
      )}
      {...props}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre({ children }) {
            return <>{children}</>
          },
          code({ className, children }) {
            const match = /language-([\w-]+)/.exec(className || "")
            const code = String(children).replace(/\n$/, "")

            if (!match) {
              return (
                <code className="rounded-md border bg-muted/55 px-1.5 py-0.5 text-[0.82em] font-medium text-foreground">
                  {children}
                </code>
              )
            }

            return <CodeBlock code={code} lang={match[1]} />
          },
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}

export { Markdown }
