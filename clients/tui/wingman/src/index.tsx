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

const TOOLS = ["bash", "read", "write", "edit", "glob", "grep", "webfetch"];
const AGENT_NAME = "WingCode";

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
	let agent = agents.find((item) => item.name === AGENT_NAME);
	if (!agent) {
		try {
			agent = await api.createAgent({
				name: AGENT_NAME,
				instructions: INSTRUCTIONS,
				tools: TOOLS,
				provider: "anthropic",
				model: "claude-sonnet-4-6",
				options: { max_tokens: 16384 },
			});
		} catch (err) {
			console.error("Error: failed to create agent.");
			console.error(String(err));
			console.error("If this is a schema mismatch, migrate your Wingman DB.");
			return;
		}
	}
	if (!agent) return;

	const session = await api.createSession(process.cwd());

	const renderer = await createCliRenderer({ exitOnCtrlC: false });
	createRoot(renderer).render(
		<SessionProvider agentID={agent.id} sessionID={session.id}>
			<App />
		</SessionProvider>,
	);
}

main();
