import { useEffect, useState, type ReactNode } from "react";
import { CheckCircleIcon, CircleNotchIcon, CodeIcon, FileTextIcon, GlobeIcon, MagnifyingGlassIcon, TerminalIcon, WarningCircleIcon, WrenchIcon } from "@phosphor-icons/react";

import { ToolDiff } from "@/components/tool-diff";
import { collapseOutput, compactInput, formatDuration, humanizeToolName, parseReadOutput, patchFiles, stripAnsi, toolSummary, toolText } from "@/lib/tool-display";
import type { ToolActivity, ToolCallPart, ToolResultPart } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";

type Props = {
	call: ToolCallPart;
	result?: ToolResultPart;
	activity?: ToolActivity;
};

type ToolView = {
	call: ToolCallPart;
	status: ToolActivity["status"];
	output: string;
	error: string;
	metadata: Record<string, unknown>;
	startedAt?: string;
	durationMs?: number;
};

function normalizeTool({ call, result, activity }: Props): ToolView {
	const status = activity?.status ?? (result ? (result.is_error ? "error" : "completed") : "pending");
	const input = activity?.input ?? call.input;
	const resultOutput = toolText(result);
	const legacyError = result?.is_error && !activity?.error ? resultOutput : "";
	const output = result ? (legacyError ? "" : resultOutput) : (activity?.output ?? "");
	return {
		call: { ...call, input },
		status,
		output: stripAnsi(output),
		error: stripAnsi(activity?.error ?? legacyError),
		metadata: { ...(result?.metadata ?? {}), ...(activity?.metadata ?? {}) },
		startedAt: activity?.started_at,
		durationMs: activity?.duration_ms,
	};
}

export function ToolActivityItem(props: Props) {
	const view = normalizeTool(props);
	if (view.call.name === "bash") return <BashTool view={view} />;
	if (["apply_patch", "edit", "write"].includes(view.call.name)) return <FileMutationTool view={view} />;
	return <CompactTool view={view} />;
}

function useDuration(view: ToolView) {
	const running = view.status === "pending" || view.status === "running";
	const [now, setNow] = useState(() => Date.now());
	useEffect(() => {
		if (!running || !view.startedAt) return;
		const timer = window.setInterval(() => setNow(Date.now()), 250);
		return () => window.clearInterval(timer);
	}, [running, view.startedAt]);
	if (view.durationMs !== undefined) return formatDuration(view.durationMs);
	if (!running || !view.startedAt) return "";
	return formatDuration(Math.max(0, now - new Date(view.startedAt).getTime()));
}

function StatusIcon({ status }: { status: ToolView["status"] }) {
	if (status === "pending" || status === "running") return <CircleNotchIcon className="size-3.5 animate-spin" />;
	if (status === "error") return <WarningCircleIcon className="size-3.5" weight="fill" />;
	return <CheckCircleIcon className="size-3.5" weight="fill" />;
}

function toolIcon(name: string) {
	if (name === "bash") return <TerminalIcon className="size-4" />;
	if (name === "read") return <FileTextIcon className="size-4" />;
	if (name === "grep" || name === "glob") return <MagnifyingGlassIcon className="size-4" />;
	if (name === "webfetch" || name === "websearch") return <GlobeIcon className="size-4" />;
	if (name === "apply_patch" || name === "edit" || name === "write") return <CodeIcon className="size-4" />;
	return <WrenchIcon className="size-4" />;
}

function sourceLabel(view: ToolView) {
	if (view.metadata.source !== "mcp") return "";
	const server = typeof view.metadata.server === "string" ? view.metadata.server : "MCP";
	const remote = typeof view.metadata.remote_name === "string" ? humanizeToolName(view.metadata.remote_name) : "";
	return remote ? `MCP · ${server} · ${remote}` : `MCP · ${server}`;
}

function ToolHeader({ view, title, changes }: { view: ToolView; title: string; changes?: { additions: number; deletions: number } }) {
	const duration = useDuration(view);
	const active = view.status === "pending" || view.status === "running";
	return (
		<div className={cn("flex min-w-0 flex-1 items-center gap-2", view.status === "error" ? "text-destructive" : active ? "text-foreground" : "text-muted-foreground")}>
			<span className="shrink-0">{toolIcon(view.call.name)}</span>
			<span className="shrink-0"><StatusIcon status={view.status} /></span>
			<span className="min-w-0 truncate text-xs font-medium">{title}</span>
			{sourceLabel(view) && <span className="hidden shrink-0 text-[11px] text-muted-foreground sm:inline">{sourceLabel(view)}</span>}
			{changes && <span className="ml-auto flex shrink-0 gap-2 font-mono text-xs"><span className="text-[var(--console-diff-add-foreground)]">+{changes.additions}</span><span className="text-[var(--console-diff-delete-foreground)]">-{changes.deletions}</span></span>}
			{duration && <span className={cn("shrink-0 text-[11px] tabular-nums", !changes && "ml-auto")}>{duration}</span>}
		</div>
	);
}

function BashTool({ view }: { view: ToolView }) {
	const [expanded, setExpanded] = useState(false);
	const output = view.output.trimEnd();
	const collapsed = collapseOutput(output, 10, 1_600);
	const visibleOutput = expanded || !collapsed.overflow ? output : collapsed.output;
	const command = typeof view.call.input.command === "string" ? view.call.input.command : "";
	const workDir = typeof view.metadata.work_dir === "string" ? view.metadata.work_dir : "";
	return (
		<div className={cn("my-2 overflow-hidden rounded-lg border bg-[var(--console-command-background)] text-[var(--console-command-foreground)]", view.status === "error" ? "border-destructive/50" : "border-[var(--console-command-border)]")}>
			<div className="flex items-center gap-2 border-b border-[var(--console-command-border)] px-3 py-2">
				<ToolHeader view={view} title={command || "Running command"} />
			</div>
			{workDir && <div className="border-b border-[var(--console-command-border)] px-3 py-1 font-mono text-[10px] text-[var(--console-command-muted)]">{workDir}</div>}
			{(output || view.error) && <pre data-scrollable tabIndex={0} className="max-h-96 overflow-auto whitespace-pre-wrap px-3 py-2 font-mono text-xs leading-5"><code>{visibleOutput}{view.error && view.error !== output ? `${output ? "\n" : ""}${view.error}` : ""}</code></pre>}
			{collapsed.overflow && <button type="button" onClick={() => setExpanded((value) => !value)} className="w-full border-t border-[var(--console-command-border)] px-3 py-1.5 text-left text-[11px] text-[var(--console-command-muted)] hover:text-[var(--console-command-foreground)]">{expanded ? "Collapse output" : "Show full output"}</button>}
		</div>
	);
}

function FileMutationTool({ view }: { view: ToolView }) {
	const files = patchFiles(view.metadata.files);
	const changes = files.reduce((total, file) => ({ additions: total.additions + file.additions, deletions: total.deletions + file.deletions }), { additions: 0, deletions: 0 });
	return (
		<div className={cn("my-2 border-l-2 pl-3", view.status === "error" ? "border-destructive" : "border-border")}>
			<div className="flex min-h-7 items-center"><ToolHeader view={view} title={toolSummary(view.call)} changes={files.length > 0 ? changes : undefined} /></div>
			{files.length > 0 && <ToolDiff files={files} />}
			{files.length === 0 && <OutputPreview view={view} />}
			{view.error && <ErrorText error={view.error} />}
		</div>
	);
}

function CompactTool({ view }: { view: ToolView }) {
	const read = view.call.name === "read" ? parseReadOutput(view.output) : undefined;
	const output = read?.body || view.output;
	const preview = collapseOutput(output.trimEnd(), 3, 420);
	const structured = view.metadata.structured_content;
	const hasDetails = Boolean(output || view.error || structured || Object.keys(view.call.input).length > 0);
	const defaultOpen = view.status === "error";
	const baseTitle = typeof view.metadata.remote_name === "string" ? humanizeToolName(view.metadata.remote_name) : toolSummary(view.call);
	const count = view.call.name === "grep" ? view.metadata.matches : view.call.name === "glob" ? view.metadata.count : view.call.name === "websearch" ? view.metadata.numResults : undefined;
	const countLabel = typeof count === "number" ? `${count} ${view.call.name === "websearch" ? "results" : count === 1 ? "match" : "matches"}` : "";
	const title = countLabel ? `${baseTitle} (${countLabel})` : baseTitle;
	if (!hasDetails) {
		return <div className="my-1.5 flex min-h-7 items-center"><ToolHeader view={view} title={title} /></div>;
	}
	return (
		<Collapsible defaultOpen={defaultOpen} className="my-1.5">
			<CollapsibleTrigger disabled={!hasDetails} className="min-h-7 py-0.5 hover:no-underline disabled:opacity-100">
				<ToolHeader view={view} title={title} />
			</CollapsibleTrigger>
			{preview.output && <pre className="ml-8 max-h-20 overflow-hidden whitespace-pre-wrap font-mono text-xs leading-5 text-muted-foreground">{preview.output}</pre>}
			{view.error && <ErrorText error={view.error} />}
			{hasDetails && <CollapsibleContent className="pb-0">
				<div className="ml-8 mt-1 grid gap-2 border-l pl-3">
					{read?.path && <div className="break-all font-mono text-xs text-muted-foreground">{read.path}</div>}
					{output && <DetailBlock label="Output">{output}</DetailBlock>}
					{structured !== undefined && <DetailBlock label="Structured output">{JSON.stringify(structured, null, 2)}</DetailBlock>}
					{Object.keys(view.call.input).length > 0 && <DetailBlock label="Input">{JSON.stringify(view.call.input, null, 2)}</DetailBlock>}
					{sourceLabel(view) && <div className="text-[11px] text-muted-foreground">{sourceLabel(view)}</div>}
				</div>
			</CollapsibleContent>}
		</Collapsible>
	);
}

function OutputPreview({ view }: { view: ToolView }) {
	const text = view.output || compactInput(view.call.input);
	if (!text) return null;
	return <pre className="mt-1 max-h-24 overflow-auto whitespace-pre-wrap font-mono text-xs leading-5 text-muted-foreground">{collapseOutput(text, 4, 500).output}</pre>;
}

function ErrorText({ error }: { error: string }) {
	return <pre className="ml-8 mt-1 whitespace-pre-wrap font-mono text-xs leading-5 text-destructive">{error}</pre>;
}

function DetailBlock({ label, children }: { label: string; children: ReactNode }) {
	return <div><div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{label}</div><pre data-scrollable tabIndex={0} className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md bg-muted/45 p-2 font-mono text-xs leading-5">{children}</pre></div>;
}
