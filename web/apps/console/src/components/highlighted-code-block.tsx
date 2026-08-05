import { useEffect, useState } from "react";
import type { HighlighterCore } from "shiki/core";
import { useTheme } from "@wingman/core/components/theme-provider";

const supportedLanguages = new Set([
  "bash",
  "css",
  "go",
  "html",
  "java",
  "javascript",
  "json",
  "markdown",
  "python",
  "rust",
  "sql",
  "text",
  "typescript",
  "yaml",
]);

const languageAliases: Record<string, string> = {
  js: "javascript",
  md: "markdown",
  py: "python",
  sh: "bash",
  shell: "bash",
  ts: "typescript",
  yml: "yaml",
};

let highlighterPromise: Promise<HighlighterCore> | null = null;

async function createHighlighter() {
  const [
    { createHighlighterCore },
    { createJavaScriptRegexEngine },
    githubDark,
    githubLight,
    gruvboxDark,
    gruvboxLight,
    dracula,
    nord,
    rosePine,
    rosePineDawn,
    bash,
    css,
    go,
    html,
    java,
    javascript,
    json,
    markdown,
    python,
    rust,
    sql,
    typescript,
    yaml,
  ] = await Promise.all([
    import("shiki/core"),
    import("shiki/engine/javascript"),
    import("shiki/themes/github-dark.mjs"),
    import("shiki/themes/github-light.mjs"),
    import("shiki/themes/gruvbox-dark-medium.mjs"),
    import("shiki/themes/gruvbox-light-medium.mjs"),
    import("shiki/themes/dracula.mjs"),
    import("shiki/themes/nord.mjs"),
    import("shiki/themes/rose-pine.mjs"),
    import("shiki/themes/rose-pine-dawn.mjs"),
    import("shiki/langs/bash.mjs"),
    import("shiki/langs/css.mjs"),
    import("shiki/langs/go.mjs"),
    import("shiki/langs/html.mjs"),
    import("shiki/langs/java.mjs"),
    import("shiki/langs/javascript.mjs"),
    import("shiki/langs/json.mjs"),
    import("shiki/langs/markdown.mjs"),
    import("shiki/langs/python.mjs"),
    import("shiki/langs/rust.mjs"),
    import("shiki/langs/sql.mjs"),
    import("shiki/langs/typescript.mjs"),
    import("shiki/langs/yaml.mjs"),
  ]);

  return createHighlighterCore({
    engine: createJavaScriptRegexEngine(),
    themes: [
      githubDark.default,
      githubLight.default,
      gruvboxDark.default,
      gruvboxLight.default,
      dracula.default,
      nord.default,
      rosePine.default,
      rosePineDawn.default,
    ],
    langs: [
      bash.default,
      css.default,
      go.default,
      html.default,
      java.default,
      javascript.default,
      json.default,
      markdown.default,
      python.default,
      rust.default,
      sql.default,
      typescript.default,
      yaml.default,
    ],
    langAlias: languageAliases,
  });
}

function getHighlighter() {
  highlighterPromise ??= createHighlighter();
  return highlighterPromise;
}

function normalizeLanguage(lang?: string) {
  if (!lang) return "text";

  const normalized = languageAliases[lang] ?? lang;
  return supportedLanguages.has(normalized) ? normalized : "text";
}

export function HighlightedCodeBlock({ code, lang }: { code: string; lang?: string }) {
  const { theme, resolvedColorMode } = useTheme();
  const [html, setHtml] = useState("");

  useEffect(() => {
    let mounted = true;
    async function highlight() {
      const highlighter = await getHighlighter();
      const syntaxTheme = theme.shiki[resolvedColorMode] ?? theme.shiki.dark ?? "github-dark";
      const result = highlighter.codeToHtml(code, {
        lang: normalizeLanguage(lang),
        theme: syntaxTheme,
      });
      if (mounted) setHtml(result);
    }
    void highlight();
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
