import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import type { Message, Usage } from "@/lib/types";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

export function timeAgo(dateStr: string): string {
	const date = new Date(dateStr);
	const now = new Date();
	const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);
	if (seconds < 60) return "just now";
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	if (days < 30) return `${days}d ago`;
	return date.toLocaleDateString();
}

export function splitModelRef(modelRef?: string) {
	const index = modelRef?.indexOf("/") ?? -1;
	if (!modelRef || index <= 0 || index === modelRef.length - 1) return { provider: "", model: "" };
	return { provider: modelRef.slice(0, index), model: modelRef.slice(index + 1) };
}

export function latestAssistantUsage(history: Message[]): Usage | undefined {
	return [...history].reverse().find((message) => message.role === "assistant" && contextTokenCount(message.usage) > 0)?.usage;
}

export function formatTokenCount(tokens: number): string {
	if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
	if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1)}k`;
	return String(tokens);
}

export function formatContextPercent(tokens: number, contextWindow?: number): string | null {
	if (!contextWindow || contextWindow <= 0) return null;
	const percent = (tokens / contextWindow) * 100;
	if (percent > 0 && percent < 1) return "<1%";
	return `${Math.round(percent)}%`;
}

export function contextTokenCount(usage?: Usage): number {
	if (!usage) return 0;
	const computed = billableInputTokens(usage) + visibleOutputTokens(usage) + (usage.reasoning_tokens ?? 0) + (usage.cached_input_tokens ?? 0) + (usage.cache_write_tokens ?? 0);
	return computed === 0 && usage.total_tokens > 0 ? usage.total_tokens : computed;
}

function billableInputTokens(usage: Usage): number {
	return Math.max(0, usage.input_tokens - (usage.cached_input_tokens ?? 0) - (usage.cache_write_tokens ?? 0));
}

function visibleOutputTokens(usage: Usage): number {
	return Math.max(0, usage.output_tokens - (usage.reasoning_tokens ?? 0));
}
