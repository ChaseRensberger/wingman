import { useState } from "react";
import { StackIcon } from "@phosphor-icons/react";

import type { CallTrace, ModelCall, Session } from "@/lib/types";
import { formatTokenCount } from "@/lib/utils";
import { Button } from "@wingman/core/components/core/button";
import { Card } from "@wingman/core/components/core/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@wingman/core/components/core/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@wingman/core/components/core/tooltip";

function formatCost(cost: number) {
	if (cost === 0) return "$0.00";
	if (Math.abs(cost) < 0.01) return `$${cost.toFixed(4)}`;
	return `$${cost.toFixed(2)}`;
}

function ContextRing({ percent }: { percent: number }) {
	const value = Math.max(0, Math.min(percent, 100));
	return (
		<svg viewBox="0 0 24 24" className="size-4 -rotate-90" aria-hidden="true">
			<circle cx="12" cy="12" r="8.5" fill="none" stroke="currentColor" strokeWidth="2" className="text-muted-foreground/20" />
			<circle
				cx="12"
				cy="12"
				r="8.5"
				fill="none"
				stroke="currentColor"
				strokeWidth="2"
				strokeLinecap="round"
				pathLength="100"
				strokeDasharray={`${value} 100`}
				className="text-foreground"
			/>
		</svg>
	);
}

function Stat({ label, value }: { label: string; value: string }) {
	return (
		<Card size="sm" className="min-w-0 gap-0 bg-muted/25 px-3 py-2">
			<div className="text-xs text-muted-foreground">{label}</div>
			<div className="mt-1 truncate text-sm font-medium" title={value}>{value}</div>
		</Card>
	);
}

function RequestManifest({ trace }: { trace?: CallTrace }) {
  if (!trace) return null;
  return <Collapsible className="mt-3 rounded-md border bg-muted/20">
    <CollapsibleTrigger className="px-2.5 py-2 text-xs font-medium hover:no-underline">Request manifest</CollapsibleTrigger>
    <CollapsibleContent className="border-t px-2.5 py-2 text-xs text-muted-foreground">
      <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
        <span>Capabilities</span><span>{trace.capabilities.thinking ? "Reasoning enabled" : "Default"}</span>
        <span>Current date</span><span>{trace.runtime.current_date ? "Injected" : "Not injected"}</span>
        <span>Reasoning summary</span><span>{trace.lowered?.reasoning_summary_auto ? "Auto" : "Not requested"}</span>
        <span>System prompt</span><span className="font-mono">{trace.system.sha256.slice(0, 12)} · {trace.system.bytes} bytes</span>
        <span>Messages</span><span>{trace.messages.count} · {Object.entries(trace.messages.part_kinds).map(([kind, count]) => `${count} ${kind}`).join(", ") || "no parts"}</span>
        <span>Tools</span><span>{trace.tools?.length ? trace.tools.map((tool) => tool.name).join(", ") : "None"}</span>
      </div>
    </CollapsibleContent>
  </Collapsible>;
}

export function SessionContextSheet({ session, calls }: { session: Session; calls: ModelCall[] }) {
  const [open, setOpen] = useState(false);
  const latest = session.latest_model_call ?? calls.at(-1);
  const contextPercent = latest?.context_percent ?? 0;
  const contextTokens = latest?.context_tokens ?? 0;
  const contextWindow = latest?.context_window;
  const hasUsage = calls.some((call) => call.total_tokens > 0 || call.input_tokens > 0 || call.output_tokens > 0);
  const hasCost = calls.length > 0 && calls.every((call) => call.cost !== undefined);
  const totalCost = calls.reduce((total, call) => total + (call.cost ?? 0), 0);
	const totalInput = calls.reduce((total, call) => total + call.input_tokens, 0);
	const totalOutput = calls.reduce((total, call) => total + call.output_tokens, 0);
	const totalReasoning = calls.reduce((total, call) => total + (call.reasoning_tokens ?? 0), 0);
	const totalCached = calls.reduce((total, call) => total + (call.cached_input_tokens ?? 0), 0);

	return (
		<Sheet open={open} onOpenChange={setOpen}>
			<Tooltip>
				<TooltipTrigger render={<Button type="button" variant="ghost" size="icon-sm" aria-label="View session context and usage" onClick={() => setOpen(true)} />}>
					<ContextRing percent={contextPercent} />
				</TooltipTrigger>
        <TooltipContent className="w-44 bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10">
          <div className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5">
            <span className="text-muted-foreground">Estimated cost</span><span>{hasCost ? formatCost(totalCost) : "Not reported"}</span>
            <span className="text-muted-foreground">Usage</span><span>{hasUsage ? `${Math.round(contextPercent)}%` : "Not reported"}</span>
            <span className="text-muted-foreground">Tokens</span><span>{hasUsage ? formatTokenCount(contextTokens) : "Not reported"}</span>
					</div>
				</TooltipContent>
			</Tooltip>
			<SheetContent className="w-full gap-4 p-0 sm:max-w-xl">
				<SheetHeader className="border-b px-5 py-4 pr-12">
				<SheetTitle>Inspector</SheetTitle>
				</SheetHeader>
				<div className="min-h-0 flex-1 overflow-y-auto px-5 pb-6">
            <div className="grid grid-cols-2 gap-2">
              <Stat label="Context" value={hasUsage ? (contextWindow ? `${formatTokenCount(contextTokens)} / ${formatTokenCount(contextWindow)}` : formatTokenCount(contextTokens)) : "Not reported"} />
              <Stat label="Context usage" value={hasUsage ? `${Math.round(contextPercent)}%` : "Not reported"} />
              <Stat label="Estimated cost" value={hasCost ? formatCost(totalCost) : "Not reported"} />
							<Stat label="Model calls" value={String(calls.length)} />
							<Stat label="Input tokens" value={hasUsage ? formatTokenCount(totalInput) : "Not reported"} />
							<Stat label="Output tokens" value={hasUsage ? formatTokenCount(totalOutput) : "Not reported"} />
							<Stat label="Reasoning tokens" value={hasUsage ? formatTokenCount(totalReasoning) : "Not reported"} />
							<Stat label="Cached tokens" value={hasUsage ? formatTokenCount(totalCached) : "Not reported"} />
						</div>
						<section className="mt-6">
							<div className="mb-2 flex items-center gap-2 text-sm font-medium"><StackIcon className="size-4" />Model calls</div>
							{calls.length === 0 ? (
								<p className="text-sm text-muted-foreground">No persisted model calls yet.</p>
							) : (
								<div className="overflow-hidden rounded-lg border">
									{calls.map((call) => (
								<div key={call.id} className="border-b px-3 py-3 last:border-b-0">
											<div className="flex items-start justify-between gap-3">
												<div className="min-w-0"><div className="truncate text-sm font-medium">{call.model_ref || call.model_id || "Unknown model"}</div><div className="mt-0.5 truncate text-xs text-muted-foreground">{call.run_id ? `${call.run_id} · ` : ""}Step {call.step} · attempt {call.attempt} · {call.status}{call.finish_reason ? ` · ${call.finish_reason}` : ""}</div></div>
											<span className="shrink-0 text-xs text-muted-foreground">{call.cost === undefined ? "Not reported" : formatCost(call.cost)}</span>
											</div>
                      {call.total_tokens > 0 || call.input_tokens > 0 || call.output_tokens > 0 ? (
                        <div className="mt-2 grid grid-cols-3 gap-2 text-xs text-muted-foreground"><span>In {formatTokenCount(call.input_tokens)}</span><span>Out {formatTokenCount(call.output_tokens)}</span><span>{Math.round(call.context_percent ?? 0)}% context</span></div>
                      ) : <div className="mt-2 text-xs text-muted-foreground">Usage not reported by provider.</div>}
											{call.error_message && <div className="mt-2 rounded bg-destructive/10 px-2 py-1 text-xs text-destructive">{call.error_message}</div>}
											{call.provider_request_id && <div className="mt-2 truncate font-mono text-xs text-muted-foreground">Provider request {call.provider_request_id}</div>}
											<RequestManifest trace={call.trace} />
										</div>
									))}
								</div>
							)}
						</section>
				</div>
			</SheetContent>
		</Sheet>
	);
}
