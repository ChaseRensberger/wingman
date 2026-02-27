import { createFileRoute } from '@tanstack/react-router'
import { useState } from "react";
import { Copy, Check } from "lucide-react";
import { Button } from "@wingman/core/components/primitives/button";
import WingmanIcon from "../assets/WingmanBlue.png";
import { Link } from '@tanstack/react-router';
import { ASCIILOGO } from '../components/ascii-logo';

export const Route = createFileRoute('/')({
	component: RouteComponent,
})

function RouteComponent() {
	return <Hero />
}

const SDK_COMMAND = "go get github.com/chaserensberger/wingman";
const SERVER_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";

function CopyCommand({ command, children }: { command: string; children: React.ReactNode }) {
	const [copied, setCopied] = useState(false);

	const handleCopy = async () => {
		await navigator.clipboard.writeText(command);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="flex items-center gap-3 bg-card border rounded-sm px-4 py-3 font-mono text-sm">
			<span className="text-muted-foreground select-none">$</span>
			<code className="flex-1 overflow-hidden text-ellipsis whitespace-nowrap scrollbar-hide text-muted-foreground">
				{children}
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

function InstallSection() {
	return (
		<div className="space-y-6">
			<div className="space-y-2">
				<p className="text-xs text-muted-foreground uppercase tracking-wider">Server</p>
				<CopyCommand command={SERVER_COMMAND}>
					{SERVER_COMMAND}
				</CopyCommand>
			</div>
			<div className="space-y-2">
				<p className="text-xs text-muted-foreground uppercase tracking-wider">SDK</p>
				<CopyCommand command={SDK_COMMAND}>
					{SDK_COMMAND}
				</CopyCommand>
			</div>
		</div >
	);
}

function NavLink(navItem: {
	name: string,
	url: string
}) {
	return (
		<Link
			to={navItem.url}
			className="text-muted-foreground hover:text-foreground transition-colors hover:underline hover:underline-offset-4"
		>
			{navItem.name}
		</Link>
	)
}

function Hero() {
	return (
		<main className="min-h-screen flex flex-col md:max-w-3xl lg:max-w-4xl mx-auto border">
			<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<img src={WingmanIcon} className="w-12 h-12" />
				<div className="flex items-center gap-6">
					<NavLink name="GitHub" url={GITHUB_URL} />
					{/*
					<NavLink name="Blog" url={"/blog"} />
					*/}
					<NavLink name="Docs" url={"/docs"} />
				</div>
			</nav>
			<section className="flex-1 border-b p-12 space-y-8">
				{/*
				<div className="bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30 text-sm text-center rounded-sm px-4 py-2 font-medium">
					This project is under active development. When you update versions, APIs may change drastically (do not expect backward compatibility).
				</div>
					*/}
				<ASCIILOGO />
				<div className="space-y-4">
					<h2 className="md:text-lg text-muted-foreground leading-relaxed">
						An open source, highly performant, actor-based, agent orchestration framework
					</h2>
					<InstallSection />
				</div>
			</section>
			<footer className="px-6 py-2 text-center">
				<p className="text-sm text-muted-foreground font-mono">
					Wingman
				</p>
			</footer>
		</main >
	);
}
