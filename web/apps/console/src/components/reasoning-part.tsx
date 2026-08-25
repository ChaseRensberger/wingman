import { CircleNotchIcon } from "@phosphor-icons/react";
import { lazy, Suspense } from "react";

import { reasoningSummary } from "@/lib/session-detail";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@wingman/core/components/core/collapsible";

const Markdown = lazy(async () => {
  const { Markdown } = await import("@/components/markdown");
  return { default: Markdown };
});

export function ReasoningPart({
  reasoning,
  isStreaming = false,
}: {
  reasoning: string;
  isStreaming?: boolean;
}) {
  const { title, body } = reasoningSummary(reasoning);
  if (!title && !body) return null;
  const header = (
    <div className="flex min-w-0 items-center gap-2 text-xs text-amber-700 dark:text-amber-300">
      {isStreaming && <CircleNotchIcon className="size-3.5 shrink-0 animate-spin" />}
      <span className="shrink-0 font-medium">{isStreaming ? "Thinking" : "Thought:"}</span>
      {title && <span className="min-w-0 truncate text-muted-foreground">{title}</span>}
    </div>
  );
  if (!body) return <div className="my-2">{header}</div>;
  return (
    <Collapsible defaultOpen={isStreaming} className="my-2">
      <CollapsibleTrigger className="min-h-7 py-0.5 text-left hover:no-underline">
        {header}
      </CollapsibleTrigger>
      {body && (
        <CollapsibleContent className="mt-1 text-sm text-muted-foreground">
          <Suspense fallback={<div className="whitespace-pre-wrap">{body}</div>}>
            <Markdown text={body} isStreaming={isStreaming} />
          </Suspense>
        </CollapsibleContent>
      )}
    </Collapsible>
  );
}
