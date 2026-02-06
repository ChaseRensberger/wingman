import { useState } from "react";
import { Copy, Check } from "lucide-react";
import { Button } from "@wingman/core/components/button";
import WingmanIcon from "./assets/WingmanBlue.png";

const INSTALL_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";
const DOCS_URL = "https://github.com/ChaseRensberger/wingman/tree/main/docs";

function CopyCommand() {
	const [copied, setCopied] = useState(false);

	const handleCopy = async () => {
		await navigator.clipboard.writeText(INSTALL_COMMAND);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="flex items-center gap-3 bg-card border rounded-sm px-4 py-3 font-mono text-sm">
			<span className="text-muted-foreground select-none">$</span>
			<code className="flex-1 overflow-hidden text-ellipsis whitespace-nowrap scrollbar-hide text-muted-foreground">
				curl -fsSL https://<span className="font-semibold text-foreground">wingman.actor/install</span> | bash
			</code>
			<Button
				variant="ghost"
				onClick={handleCopy}
				className="text-muted-foreground hover:text-foreground transition-colors p-1 -m-1 shrink-0"
				aria-label="Copy install command"
			>
				{copied ? (
					<Check className="h-4 w-4 text-green-500" />
				) : (
					<Copy className="h-4 w-4" />
				)}
			</Button>
		</div>
	);
}

function NavLink(navItem: {
	name: string,
	url: string
}) {
	return (
		<a
			href={navItem.url}
			className="text-muted-foreground hover:text-foreground transition-colors hover:underline"
		>
			{navItem.name}
		</a>
	)
}

export default function App() {
	return (
		<main className="min-h-screen flex flex-col md:max-w-2xl lg:max-w-4xl mx-auto border">
			<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<img src={WingmanIcon} className="w-12 h-12" />
				<div className="flex items-center gap-6">
					<NavLink name="GitHub" url={GITHUB_URL} />
					<NavLink name="Docs" url={DOCS_URL} />
				</div>
			</nav>
			<section className="flex-1 border-b p-12 space-y-8">
				<h1 className="text-4xl text-primary font-semibold text-center tracking-widest">WINGMAN</h1>
				<div className="space-y-4">
					<h2 className="text-lg text-muted-foreground leading-relaxed text-balance">
						An open source, highly performant, actor-based, agent orchestration framework
					</h2>
					<CopyCommand />
				</div>
			</section>
			<footer className="px-6 py-2 text-center">
				<p className="text-sm text-muted-foreground font-mono">
					Hero
				</p>
			</footer>
		</main >
	);
}
