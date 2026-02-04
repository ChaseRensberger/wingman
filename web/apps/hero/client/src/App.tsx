import { useState } from "react";
import { Copy, Check } from "lucide-react";

const INSTALL_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";
const DOCS_URL = "https://docs.wingman.actor";

function CopyCommand() {
	const [copied, setCopied] = useState(false);

	const handleCopy = async () => {
		await navigator.clipboard.writeText(INSTALL_COMMAND);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="relative group">
			<div className="flex items-center gap-3 bg-card border border-border rounded-lg px-4 py-3 font-mono text-sm">
				<span className="text-muted-foreground select-none">$</span>
				<code className="text-foreground flex-1 overflow-x-auto whitespace-nowrap scrollbar-hide">
					{INSTALL_COMMAND}
				</code>
				<button
					onClick={handleCopy}
					className="text-muted-foreground hover:text-foreground transition-colors p-1 -m-1 shrink-0"
					aria-label="Copy install command"
				>
					{copied ? (
						<Check className="h-4 w-4 text-green-500" />
					) : (
						<Copy className="h-4 w-4" />
					)}
				</button>
			</div>
		</div>
	);
}

export default function App() {
	return (
		<main className="min-h-screen flex flex-col">
			<nav className="flex items-center justify-between px-6 py-4 w-full border-b">
				<span className="font-mono text-muted-foreground">
					wingman
				</span>
				<div className="flex items-center gap-6">
					<a
						href={GITHUB_URL}
						className="text-sm text-muted-foreground hover:text-foreground transition-colors"
					>
						github
					</a>
					<a
						href={DOCS_URL}
						className="text-sm text-muted-foreground hover:text-foreground transition-colors"
					>
						docs
					</a>
				</div>
			</nav>
			<div className="flex-1 flex flex-col items-center justify-center px-6 pb-20">
				<div className="max-w-2xl w-full space-y-8 text-center">
					<div className="space-y-4">
						<p className="text-muted-foreground text-lg sm:text-xl leading-relaxed text-balance">
							highly performant actor-based agent orchestration
						</p>
					</div>
					<CopyCommand />
				</div>
			</div>

			<footer className="px-6 py-4 text-center">
				<p className="text-xs text-muted-foreground font-mono">
					Hero
				</p>
			</footer>
		</main>
	);
}
