import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

const DEFAULT_WINGMAN_URL = "http://localhost:2323";

export function loadEnvLocal() {
	const path = resolve(process.cwd(), ".env.local");
	if (!existsSync(path)) return;

	const contents = readFileSync(path, "utf8");
	for (const line of contents.split(/\r?\n/)) {
		const trimmed = line.trim();
		if (!trimmed || trimmed.startsWith("#")) continue;

		const eqIndex = trimmed.indexOf("=");
		if (eqIndex === -1) continue;

		const key = trimmed.slice(0, eqIndex).trim();
		let value = trimmed.slice(eqIndex + 1).trim();
		if (!key) continue;

		if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
			value = value.slice(1, -1);
		}

		if (process.env[key] === undefined) {
			process.env[key] = value;
		}
	}
}

export function getWingmanUrl() {
	return process.env.WINGMAN_URL || DEFAULT_WINGMAN_URL;
}
