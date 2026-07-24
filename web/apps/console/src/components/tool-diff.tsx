import { FileIcon } from "@phosphor-icons/react";

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";
import { cn } from "@/lib/utils";
import { parseUnifiedDiff, type PatchFile } from "@/lib/tool-display";

function operationLabel(type: string) {
	if (type === "add") return "Added";
	if (type === "delete") return "Deleted";
	if (type === "move") return "Moved";
	return "Modified";
}

export function ToolDiff({ files }: { files: PatchFile[] }) {
	return (
		<div className="mt-2 overflow-hidden rounded-lg border bg-background">
			{files.map((file, index) => (
				<Collapsible key={`${file.relativePath}:${index}`} defaultOpen={files.length === 1 || index === 0} className="border-b last:border-b-0">
					<CollapsibleTrigger className="rounded-none px-3 py-2 hover:no-underline">
						<div className="flex min-w-0 flex-1 items-center gap-2">
							<FileIcon className="size-4 shrink-0 text-muted-foreground" />
							<span className="truncate font-mono text-xs">{file.relativePath}</span>
							{file.movePath && <span className="truncate text-xs text-muted-foreground">→ {file.movePath}</span>}
							<span className="ml-auto shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{operationLabel(file.type)}</span>
							<span className="shrink-0 font-mono text-xs text-emerald-600 dark:text-emerald-400">+{file.additions}</span>
							<span className="shrink-0 font-mono text-xs text-red-600 dark:text-red-400">-{file.deletions}</span>
						</div>
					</CollapsibleTrigger>
					<CollapsibleContent className="pb-0">
						<DiffTable patch={file.patch} />
					</CollapsibleContent>
				</Collapsible>
			))}
		</div>
	);
}

function DiffTable({ patch }: { patch: string }) {
	const lines = parseUnifiedDiff(patch);
	return (
		<div data-scrollable tabIndex={0} className="max-h-[32rem] overflow-auto border-t bg-muted/15 font-mono text-xs leading-5">
			<div className="min-w-max">
				{lines.map((line, index) => {
					if (line.kind === "header") return null;
					if (line.kind === "hunk" || line.kind === "note") {
						return <div key={index} className="border-y bg-muted/50 px-3 py-1 text-muted-foreground first:border-t-0">{line.text}</div>;
					}
					return (
						<div key={index} className={cn("grid grid-cols-[3.25rem_3.25rem_1fr]", line.kind === "addition" && "bg-emerald-500/12 text-emerald-950 dark:text-emerald-100", line.kind === "deletion" && "bg-red-500/12 text-red-950 dark:text-red-100")}>
							<span className="select-none border-r px-2 text-right text-muted-foreground/70">{line.oldLine ?? ""}</span>
							<span className="select-none border-r px-2 text-right text-muted-foreground/70">{line.newLine ?? ""}</span>
							<code className="whitespace-pre px-3"><span className="mr-2 inline-block w-2 select-none text-muted-foreground">{line.kind === "addition" ? "+" : line.kind === "deletion" ? "-" : " "}</span>{line.text || " "}</code>
						</div>
					);
				})}
			</div>
		</div>
	);
}
