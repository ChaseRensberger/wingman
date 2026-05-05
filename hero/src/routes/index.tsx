import { createFileRoute } from '@tanstack/react-router'
import { useState } from "react";
import { Copy, Check } from "@phosphor-icons/react";
import { Button } from "@/components/core/button";
import WingmanIcon from "../assets/WingmanBlue.png";
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
const DOCS_URL = "https://wingman.actor/docs";
const DISCORD_URL = "https://discord.gg/Sxt68YGuZu";

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
				size="icon-sm"
				onClick={handleCopy}
				className="shrink-0"
				aria-label="Copy install command"
			>
				{copied ? (
					<Check className="size-4 text-green-500" weight="bold" />
				) : (
					<Copy className="size-4" />
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
		<a
			href={navItem.url}
			className="text-muted-foreground hover:text-foreground transition-colors hover:underline hover:underline-offset-4"
		>
			{navItem.name}
		</a>
	)
}

function Hero() {
	return (
		<main className="min-h-screen flex flex-col md:max-w-4xl lg:max-w-5xl mx-auto border">
			<nav className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<img src={WingmanIcon} className="w-12 h-12" />
				<div className="flex items-center gap-6">
					<NavLink name="GitHub" url={GITHUB_URL} />
					<NavLink name="Docs" url={DOCS_URL} />
					<NavLink name="Discord" url={DISCORD_URL} />
				</div>
			</nav>
			<section className="border-b p-12 space-y-8">
				<div className="bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30 text-sm text-center rounded-sm px-4 py-2 font-medium">
					This project is under active development and not ready for production use.
				</div>
				<div className="space-y-2">
					<div className="overflow-y-hidden mx-auto w-full max-w-full overflow-x-auto text-[0.5rem] sm:text-[0.6rem] md:text-[0.7rem] lg:text-[0.875rem]">
						<ASCIILOGO />
					</div>
					<p className="text-center text-muted-foreground font-medium">
						The open-source client-agnostic agent harness
					</p>
				</div>
				<div className="space-y-4">
					<InstallSection />
				</div>
			</section>
			<section className='flex-1 px-12 py-4 border-b space-y-4'>
				<h2 className='font-extrabold text-lg'>Core</h2>
				<ul className="text-muted-foreground space-y-2">
					<li><span className="text-primary">[*]</span> <a className="hover:text-primary" href="/">WingAgent - Core agent runtime</a></li>
					<li><span className="text-primary">[*]</span> <a className="hover:text-primary" href="https://models.wingman.actor">WingModels - A provider agnostic model api</a></li>
					<li><span className="text-primary">[*]</span> <a className="hover:text-primary" href="https://news.wingman.actor">WingNews - A HackerNews client</a></li>
				</ul>
			</section >
			<footer className="px-6 py-4 text-center">
				<p className="text-sm text-muted-foreground font-mono">
					Wingman
				</p>
			</footer>
		</main >
	);
}
