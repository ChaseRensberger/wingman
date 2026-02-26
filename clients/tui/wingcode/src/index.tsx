import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { api } from "./api";
import { App } from "./app";
import { SessionProvider } from "./context/session";
import { loadEnvLocal } from "./config";

const INSTRUCTIONS = `You are a helpful coding assistant called WingCode. You help users write, edit, and understand code.

Be concise and direct. When writing code:
- Use the write tool for new files
- Use the edit tool for modifying existing files
- Use the bash tool for running commands
- Use the read tool to examine files
- Use glob and grep to search the codebase

Always explain what you're doing briefly. Follow existing code conventions.`;

const TOOLS = ["bash", "read", "write", "edit", "glob", "grep", "webfetch", "perplexity_search"];
const AGENT_NAME = "WingCode";
const AGENT_PROVIDER = "ollama";
const AGENT_MODEL = "lfm2";
const AGENT_OPTIONS = { max_tokens: 16384 };

async function main() {
	loadEnvLocal();

	try {
		await api.health();
	} catch {
		console.error("Error: wingman server not running.");
		console.error("Set WINGMAN_URL in .env.local or start with: wingman serve");
		return;
	}

	const agents = await api.listAgents();
	const existingAgent = agents.find((item) => item.name === AGENT_NAME);
	let agentID = existingAgent?.id;
	if (!existingAgent) {
		try {
			const created = await api.createAgent({
				name: AGENT_NAME,
				instructions: INSTRUCTIONS,
				tools: TOOLS,
				provider: AGENT_PROVIDER,
				model: AGENT_MODEL,
				options: AGENT_OPTIONS,
			});
			agentID = created.id;
		} catch (err) {
			console.error("Error: failed to create agent.");
			console.error(String(err));
			console.error("If this is a schema mismatch, migrate your Wingman DB.");
			return;
		}
	} else {
		try {
			await api.updateAgent(existingAgent.id, {
				provider: AGENT_PROVIDER,
				model: AGENT_MODEL,
				options: AGENT_OPTIONS,
			});
		} catch (err) {
			console.error("Error: failed to update WingCode agent model.");
			console.error(String(err));
			return;
		}
	}
	if (!agentID) return;

	const session = await api.createSession(process.cwd());

	const renderer = await createCliRenderer({ exitOnCtrlC: true });
	createRoot(renderer).render(
		<SessionProvider agentID={agentID} sessionID={session.id}>
			<App />
		</SessionProvider>,
	);
}

main();
