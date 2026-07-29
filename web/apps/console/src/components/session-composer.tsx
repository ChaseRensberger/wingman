import { ArrowDownIcon, PaperPlaneIcon, StopIcon } from "@phosphor-icons/react";
import { type RefObject } from "react";

import type { Agent, Provider, ProviderModel } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@wingman/core/components/core/select";
import { Textarea } from "@wingman/core/components/core/textarea";

type Props = {
	composerRef: RefObject<HTMLTextAreaElement | null>;
	messageText: string;
	selectedAgent: string;
	selectedAgentName?: string;
	selectedProvider: string;
	selectedModel: string;
	selectedProviderName?: string;
	agents: Agent[];
	providers: Provider[];
	models: Record<string, ProviderModel[]>;
	hasModels: boolean;
	isStreaming: boolean;
	isNearTranscriptBottom: boolean;
	onMessageChange: (value: string) => void;
	onAgentChange: (agentId: string) => void;
	onModelChange: (modelRef: string) => void;
	onSubmit: () => void;
	onAbort: () => void;
	onJumpToBottom: () => void;
};

export function SessionComposer(props: Props) {
	const modelValue = props.selectedProvider && props.selectedModel ? `${props.selectedProvider}/${props.selectedModel}` : "";
	const modelLabel = props.selectedProviderName && props.selectedModel ? `${props.selectedProviderName} / ${props.selectedModel}` : undefined;
	return (
		<form onSubmit={(event) => { event.preventDefault(); props.onSubmit(); }} className="shrink-0 px-3 pb-3 sm:px-4 sm:pb-4">
			<div className="relative mx-auto max-w-4xl rounded-xl border bg-card p-2 shadow-lg shadow-primary/10">
				{!props.isNearTranscriptBottom && <Button className="absolute -top-10 right-2 z-20 rounded-full shadow-md" size="icon-sm" type="button" onClick={props.onJumpToBottom} aria-label="Jump to latest message" title="Jump to latest message"><ArrowDownIcon className="size-4" /></Button>}
				<Textarea ref={props.composerRef} value={props.messageText} onChange={(event) => props.onMessageChange(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); props.onSubmit(); } }} placeholder="Ask anything..." className="min-h-20 max-h-60 resize-none overflow-y-auto border-0 bg-transparent shadow-none focus-visible:ring-0 sm:min-h-24" disabled={props.isStreaming} />
				<div className="mt-2 flex items-center justify-between gap-2 border-t pt-2">
					<div className="flex min-w-0 flex-wrap items-center gap-1.5">
						<Select value={props.selectedAgent} onValueChange={(value) => props.onAgentChange(value ?? "")}><SelectTrigger className="h-8 w-40 border-0 bg-muted/60 text-xs shadow-none sm:w-56"><SelectValue placeholder="Select agent">{props.selectedAgentName}</SelectValue></SelectTrigger><SelectContent>{props.agents.map((agent) => <SelectItem key={agent.id} value={agent.id}>{agent.name}</SelectItem>)}</SelectContent></Select>
						<Select value={modelValue} onValueChange={(value) => props.onModelChange(value ?? "")} disabled={!props.hasModels}><SelectTrigger className="h-8 w-44 border-0 bg-muted/60 text-xs shadow-none sm:w-72"><SelectValue placeholder="Select model">{modelLabel}</SelectValue></SelectTrigger><SelectContent>{props.providers.map((provider) => <SelectGroup key={provider.id}><SelectLabel>{provider.name}</SelectLabel>{(props.models[provider.id] ?? []).map((model) => <SelectItem key={`${provider.id}/${model.id}`} value={`${provider.id}/${model.id}`}>{model.id}</SelectItem>)}</SelectGroup>)}</SelectContent></Select>
					</div>
					<div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
						{props.isStreaming ? <Button size="icon-sm" variant="destructive" type="button" onClick={props.onAbort} aria-label="Stop generation" title="Stop generation"><StopIcon className="size-4" /></Button> : <Button size="icon-sm" type="submit" aria-label="Send message" title="Send message" disabled={!props.messageText.trim() || !props.selectedAgent}><PaperPlaneIcon className="size-4" /></Button>}
					</div>
				</div>
			</div>
		</form>
	);
}
