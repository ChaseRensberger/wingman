import { lazy, Suspense } from "react";
import { Collapsible as CollapsiblePrimitive } from "@base-ui/react/collapsible";

import { reasoningSummary } from "@/lib/session-detail";
import { Collapsible, CollapsibleContent } from "@wingman/core/components/core/collapsible";
import { Spinner } from "@wingman/core/components/core/spinner";

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
    <div className="flex min-w-0 items-center gap-2 text-xs text-primary">
      {isStreaming && <Spinner size={16} className="size-4 shrink-0" label="Thinking" />}
      <span className="shrink-0 font-medium transition-colors group-hover/thought:text-foreground">
        {isStreaming ? "Thinking" : "Thought:"}
      </span>
      {title && (
        <span className="min-w-0 truncate text-muted-foreground transition-colors group-hover/thought:text-foreground">
          {title}
        </span>
      )}
    </div>
  );
  if (!body) return <div className="flex min-h-7 items-center py-0.5">{header}</div>;
  return (
    <Collapsible>
      <CollapsiblePrimitive.Trigger className="group/thought flex min-h-7 max-w-full items-center rounded-sm py-0.5 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
        {header}
      </CollapsiblePrimitive.Trigger>
      {body && (
        <CollapsibleContent className="pt-1 pb-0 text-sm text-muted-foreground">
          <Suspense fallback={<div className="whitespace-pre-wrap">{body}</div>}>
            <Markdown text={body} isStreaming={isStreaming} />
          </Suspense>
        </CollapsibleContent>
      )}
    </Collapsible>
  );
}
