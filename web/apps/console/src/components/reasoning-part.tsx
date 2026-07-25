import { BrainIcon, CircleNotchIcon } from "@phosphor-icons/react";

import { Markdown } from "@/components/markdown";
import { reasoningSummary } from "@/lib/session-detail";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";

export function ReasoningPart({ reasoning, isStreaming = false }: { reasoning: string; isStreaming?: boolean }) {
	const { title, body } = reasoningSummary(reasoning);
	if (!title && !body) return null;
	return (
		<Collapsible defaultOpen={isStreaming} className="my-2 border-l-2 border-amber-500/45 pl-3">
			<CollapsibleTrigger className="min-h-7 py-0.5 text-left hover:no-underline">
				<div className="flex min-w-0 items-center gap-2 text-xs text-amber-700 dark:text-amber-300">
					{isStreaming ? <CircleNotchIcon className="size-3.5 shrink-0 animate-spin" /> : <BrainIcon className="size-3.5 shrink-0" />}
					<span className="shrink-0 font-medium">{isStreaming ? "Thinking" : "Thought"}</span>
					{title && <span className="min-w-0 truncate text-muted-foreground">{title}</span>}
				</div>
			</CollapsibleTrigger>
			{body && <CollapsibleContent className="ml-5 mt-1 border-l border-border/60 pl-3 text-sm text-muted-foreground"><Markdown text={body} isStreaming={isStreaming} /></CollapsibleContent>}
		</Collapsible>
	);
}
