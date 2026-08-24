import { ArrowDownIcon, PaperPlaneIcon, StopIcon } from "@phosphor-icons/react";
import { type RefObject } from "react";

import type { Agent, Provider, ProviderModel } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";
import { Card } from "@wingman/core/components/core/card";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from "@wingman/core/components/core/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@wingman/core/components/core/select";
import { Textarea } from "@wingman/core/components/core/textarea";

type Props = {
	composerRef: RefObject<HTMLTextAreaElement | null>;
	messageText: string;
	selectedAgent: string;
	selectedAgentName?: string;
	selectedProvider: string;
	selectedModel: string;
	selectedVariant: string | null;
	selectedProviderName?: string;
	agents: Agent[];
	providers: Provider[];
	models: Record<string, ProviderModel[]>;
	hasModels: boolean;
	isStreaming: boolean;
	isNearTranscriptBottom: boolean;
	onMessageChange: (value: string) => void;
	onAgentChange: (agentId: string) => void;
	onModelChange: (modelRef: string, variant?: string | null) => void;
	onSubmit: () => void;
	onAbort: () => void;
	onJumpToBottom: () => void;
};

export function SessionComposer(props: Props) {
	const modelLabel = props.selectedProviderName && props.selectedModel ? `${props.selectedProviderName} / ${props.selectedModel}${props.selectedVariant ? ` · ${props.selectedVariant}` : ""}` : "Select model";
	return (
		<form onSubmit={(event) => { event.preventDefault(); props.onSubmit(); }} className="shrink-0 px-3 pb-3 sm:px-4 sm:pb-4">
			<Card size="sm" className="relative mx-auto max-w-4xl gap-0 p-2">
				{!props.isNearTranscriptBottom && <Button className="absolute -top-10 right-2 z-20 rounded-full shadow-md" size="icon-sm" type="button" onClick={props.onJumpToBottom} aria-label="Jump to latest message" title="Jump to latest message"><ArrowDownIcon className="size-4" /></Button>}
				<Textarea ref={props.composerRef} value={props.messageText} onChange={(event) => props.onMessageChange(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); props.onSubmit(); } }} placeholder="Ask anything..." className="min-h-20 max-h-60 resize-none overflow-y-auto border-0 bg-transparent shadow-none focus-visible:ring-0 sm:min-h-24" disabled={props.isStreaming} />
				<div className="mt-2 flex items-center justify-between gap-2 border-t pt-2">
					<div className="flex min-w-0 flex-wrap items-center gap-1.5">
						<Select value={props.selectedAgent} onValueChange={(value) => props.onAgentChange(value ?? "")}><SelectTrigger className="h-8 w-40 border-0 bg-muted/60 text-xs shadow-none sm:w-56"><SelectValue placeholder="Select agent">{props.selectedAgentName}</SelectValue></SelectTrigger><SelectContent>{props.agents.map((agent) => <SelectItem key={agent.id} value={agent.id}>{agent.name}</SelectItem>)}</SelectContent></Select>
						<DropdownMenu><DropdownMenuTrigger render={<Button type="button" variant="ghost" disabled={!props.hasModels} className="h-8 w-44 justify-between border-0 bg-muted/60 px-2 text-xs font-normal shadow-none sm:w-72" />}>{modelLabel}</DropdownMenuTrigger><DropdownMenuContent className="max-h-80 w-72">{props.providers.map((provider) => <DropdownMenuGroup key={provider.id}><DropdownMenuLabel>{provider.name}</DropdownMenuLabel>{(props.models[provider.id] ?? []).map((model) => { const modelRef = `${provider.id}/${model.id}`; return model.variants?.length ? <DropdownMenuSub key={modelRef}><DropdownMenuSubTrigger>{model.id}</DropdownMenuSubTrigger><DropdownMenuSubContent><DropdownMenuItem onClick={() => props.onModelChange(modelRef, null)}>Provider default</DropdownMenuItem>{model.variants.map((variant) => <DropdownMenuItem key={variant} onClick={() => props.onModelChange(modelRef, variant)}>{variant}</DropdownMenuItem>)}</DropdownMenuSubContent></DropdownMenuSub> : <DropdownMenuItem key={modelRef} onClick={() => props.onModelChange(modelRef, null)}>{model.id}</DropdownMenuItem>; })}</DropdownMenuGroup>)}</DropdownMenuContent></DropdownMenu>
					</div>
					<div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
						{props.isStreaming ? <Button size="icon-sm" variant="destructive" type="button" onClick={props.onAbort} aria-label="Stop generation" title="Stop generation"><StopIcon className="size-4" /></Button> : <Button size="icon-sm" type="submit" aria-label="Send message" title="Send message" disabled={!props.messageText.trim() || !props.selectedAgent}><PaperPlaneIcon className="size-4" /></Button>}
					</div>
				</div>
			</Card>
		</form>
	);
}
