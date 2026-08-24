import type { Agent, Message, Part, ProviderModel, Session } from "@/lib/types";
import { splitModelRef } from "@/lib/utils";

export const LAST_AGENT_ID_KEY = "wingman_last_agent_id";
export const LAST_MODEL_REF_KEY = "wingman_last_model_ref";
export const MODEL_VARIANTS_KEY = "wingman_model_variants";
export const DEFAULT_SESSION_TITLE = "New session";

export function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function buildUserMessage(text: string): Message {
	return { role: "user", content: [{ type: "text", text } as Part] };
}

export function withFailedUserMessage(messages: Message[], text?: string): Message[] {
	if (!text) return messages;
	const latestUser = messages.findLast((message) => message.role === "user");
	const latestText = latestUser?.content
		.filter((part) => part.type === "text")
		.map((part) => (part as { text: string }).text)
		.join("\n");
	return latestText === text ? messages : [...messages, buildUserMessage(text)];
}

function cleanReasoningHeading(value: string): string {
	return value.replace(/`([^`]+)`/g, "$1").replace(/\[([^\]]+)\]\([^)]+\)/g, "$1").replace(/[*_~]+/g, "").replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim().slice(0, 120);
}

export function reasoningSummary(reasoning: string) {
	const markdown = reasoning.replace(/\r\n?/g, "\n").trim();
	const boldHeading = markdown.match(/^\*\*([^*\n]+)\*\*(?:\n\s*\n|$)/);
	if (boldHeading) {
		return { title: cleanReasoningHeading(boldHeading[1] ?? ""), body: markdown.slice(boldHeading[0].length).trim() };
	}
	const heading = markdown.match(/^(?:<h[1-6][^>]*>([\s\S]*?)<\/h[1-6]>|\s{0,3}#{1,6}[ \t]+(.+?)(?:[ \t]+#+[ \t]*)?)\n?/);
	if (heading) {
		return { title: cleanReasoningHeading(heading[1] ?? heading[2] ?? ""), body: markdown.slice(heading[0].length).trim() };
	}
	return { title: "", body: markdown };
}

export function shouldShowThinking(isStreaming: boolean, hasVisibleActivity: boolean) {
	return isStreaming && !hasVisibleActivity;
}

export function modelRefExists(models: Record<string, ProviderModel[]>, modelRef: string): boolean {
	const ref = splitModelRef(modelRef);
	const model = models[ref.provider]?.find((item) => item.id === ref.model);
	return Boolean(model && (!ref.variant || model.variants?.includes(ref.variant)));
}

export function storedModelVariant(provider: string, model: string): string | null {
	if (!provider || !model) return null;
	const raw = localStorage.getItem(MODEL_VARIANTS_KEY);
	if (!raw) return null;
	try {
		const variants = JSON.parse(raw);
		if (!isRecord(variants)) return null;
		const value = variants[`${provider}/${model}`];
		return typeof value === "string" ? value : null;
	} catch {
		return null;
	}
}

export function persistModelVariant(provider: string, model: string, variant: string | null) {
	if (!provider || !model) return;
	let variants: Record<string, string> = {};
	try {
		const parsed = JSON.parse(localStorage.getItem(MODEL_VARIANTS_KEY) ?? "{}");
		if (isRecord(parsed)) {
			variants = Object.fromEntries(Object.entries(parsed).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
		}
	} catch {
		variants = {};
	}
	const key = `${provider}/${model}`;
	if (variant) variants[key] = variant;
	else delete variants[key];
	localStorage.setItem(MODEL_VARIANTS_KEY, JSON.stringify(variants));
}

export function agentExists(agents: Agent[], agentId: string): boolean {
	return Boolean(agentId && agents.some((agent) => agent.id === agentId));
}

export function persistLastAgentId(agentId: string) {
	if (agentId) localStorage.setItem(LAST_AGENT_ID_KEY, agentId);
}

export function persistLastModelRef(modelRef: string) {
	if (modelRef) localStorage.setItem(LAST_MODEL_REF_KEY, modelRef);
}

export function formatSessionError(err: unknown): string {
	const message = String(err instanceof Error ? err.message : err);
	if (message.includes("requires a working directory, but session has none")) {
		return "This session has no working directory. Set its working directory before using this agent.";
	}
	return message.replace(/^Error:\s*/, "");
}

export function shouldAutoGenerateTitle(session: Session | null): boolean {
	if (!session || session.history.length > 0) return false;
	const title = (session.title ?? "").trim();
	return title === "" || title === DEFAULT_SESSION_TITLE;
}
