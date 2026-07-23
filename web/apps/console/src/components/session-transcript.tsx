import { ArrowClockwiseIcon, CheckIcon, CopyIcon, WarningCircleIcon } from "@phosphor-icons/react";

import WingmanIcon from "@/assets/icon-128.png";
import { ChatMessage } from "@/components/chat-message";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { RawMessages } from "@/components/raw-messages";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import type { Message, ToolActivity, ToolCallPart, ToolResultPart } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";

type FailedRun = { message: string; agentId: string; modelRef: string; error: string };

type Props = {
	messages: Message[];
	rawMessages: Message[];
	jsonMode: boolean;
	greeting: string;
	streamingText: string;
	isStreaming: boolean;
	reasoningHeading: string;
	toolCallsById: Map<string, ToolCallPart>;
	toolResultsById: Map<string, ToolResultPart>;
	toolActivitiesById: Map<string, ToolActivity>;
	failedRun: FailedRun | null;
	copiedFailedRunError: boolean;
	onCopyFailedRunError: () => void;
	onRetry: () => void;
	scroll: ReturnType<typeof useTranscriptScroll>;
};

function ThinkingIndicator({ summary }: { summary: string }) {
	return (
		<div className="flex min-h-5 items-center gap-2 px-4 py-2 text-sm font-medium text-muted-foreground">
			<HexWaveSpinner size={16} className="size-3.5 shrink-0" label="Thinking" />
			<span>Thinking</span>
			{summary && <span className="min-w-0 truncate font-normal text-muted-foreground/80">{summary}</span>}
		</div>
	);
}

export function SessionTranscript({
	messages,
	rawMessages,
	jsonMode,
	greeting,
	streamingText,
	isStreaming,
	reasoningHeading,
	toolCallsById,
	toolResultsById,
	toolActivitiesById,
	failedRun,
	copiedFailedRunError,
	onCopyFailedRunError,
	onRetry,
	scroll,
}: Props) {
	return (
		<div className="relative min-h-0 flex-1" onPointerEnter={() => scroll.setIsHovered(true)} onPointerLeave={() => scroll.setIsHovered(false)}>
			<div ref={scroll.scrollRef} onScroll={scroll.handleScroll} onKeyDown={scroll.handleKeyDown} tabIndex={0} role="region" aria-label="Session transcript" data-scrollable className="h-full overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
				<div className="mx-auto flex min-h-full w-full max-w-4xl flex-col px-3 pt-5 pb-0 sm:px-4 sm:pt-6">
					{jsonMode ? (
						<div className="px-4 pb-6 sm:px-6"><RawMessages messages={rawMessages} /></div>
					) : messages.length === 0 && !streamingText ? (
						<div className="flex flex-1 items-start justify-center pt-[20dvh] pb-12 text-center"><div className="flex flex-col items-center gap-4"><img src={WingmanIcon} className="size-16" alt="Wingman logo" /><div className="text-2xl font-semibold sm:text-3xl">{greeting}</div></div></div>
					) : (
						<div>
							{messages.map((message, index) => <ChatMessage key={index} message={message} toolCallsById={toolCallsById} toolResultsById={toolResultsById} toolActivitiesById={toolActivitiesById} />)}
							{streamingText && <ChatMessage message={{ role: "assistant", content: [{ type: "text", text: streamingText }] }} isStreaming />}
							{isStreaming && <ThinkingIndicator summary={reasoningHeading} />}
							{failedRun && <div className="mx-4 my-5 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm sm:mx-6"><div className="flex items-start gap-2"><WarningCircleIcon className="mt-0.5 size-4 shrink-0 text-destructive" weight="fill" /><div className="min-w-0 flex-1"><div className="font-medium text-destructive">Message failed</div><pre data-scrollable tabIndex={0} className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap font-sans text-xs text-muted-foreground">{failedRun.error}</pre></div></div><div className="mt-3 flex justify-end gap-2"><Button size="sm" variant="ghost" type="button" onClick={onCopyFailedRunError}>{copiedFailedRunError ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}{copiedFailedRunError ? "Copied" : "Copy error"}</Button><Button size="sm" type="button" onClick={onRetry} disabled={isStreaming}><ArrowClockwiseIcon className="size-4" />Retry</Button></div></div>}
						</div>
					)}
				</div>
			</div>
			{scroll.scrollbar.height > 0 && <div className={`absolute right-0 z-10 w-3 select-none transition-opacity duration-200 ${scroll.isHovered || scroll.isScrolling || scroll.isScrollbarDragging ? "opacity-100" : "pointer-events-none opacity-0"}`} style={{ height: scroll.scrollbar.height, top: scroll.scrollbar.top }} onPointerDown={scroll.handleScrollbarPointerDown} onPointerMove={scroll.handleScrollbarPointerMove} onPointerUp={scroll.handleScrollbarPointerUp} onPointerCancel={scroll.handleScrollbarPointerUp}><div className="absolute inset-y-0 left-1/2 w-1 -translate-x-1/2 rounded-full bg-border/70 transition-colors hover:bg-foreground/40" /></div>}
		</div>
	);
}
