import { createFileRoute } from '@tanstack/react-router'
import { useState } from "react";
import { CopyIcon, CheckIcon } from "@phosphor-icons/react";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "@/components/core/accordion";
import { Button } from "@/components/core/button";
import WingmanIcon from "../assets/WingmanBlue.png";
import { ASCIILOGO } from '../components/ascii-logo';

export const Route = createFileRoute('/')({
	component: RouteComponent,
})

function RouteComponent() {
	return <Hero />
}

const SERVER_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";
const DOCS_URL = "https://docs.wingman.actor";
// const DISCORD_URL = "";
const COMPACTION_PLUGIN_URL = "https://github.com/ChaseRensberger/wingman/blob/main/plugins/compaction/compaction.go";
const WEB_CLIENT_URL = "https://github.com/ChaseRensberger/wingman/tree/main/web";
const PROVIDERS = [
	{
		name: "Anthropic",
		href: "https://github.com/ChaseRensberger/wingman/tree/main/models/catalog/providers/anthropic",
	},
	{
		name: "OpenAI",
		href: "https://github.com/ChaseRensberger/wingman/tree/main/models/catalog/providers/openai",
	},
	{
		name: "OpenCode Zen",
		href: "https://github.com/ChaseRensberger/wingman/tree/main/models/catalog/providers/opencode",
	},
];

const FAQS = [
	{
		question: "I don't get it, what is this?",
		answer:
			"Most agentic harnesses are built for a specific purpose (right now mostly coding TUIs). Wingman aims to be fully agnostic to any specific agentic use case and in turn allow consumers to build arbitrary clients on top of it."
	},
	{
		question: "Is it production-ready?",
		answer:
			"Nope! The project is under active development and it is safe to expect breaking changes. Working hard to make sure this isn't true for long though."
	},
];

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
					<CheckIcon className="size-4 text-green-500" weight="bold" />
				) : (
					<CopyIcon className="size-4" />
				)}
			</Button>
		</div>
	);
}

function InstallSection() {
	return (
		<div className="space-y-3">
			<p className="text-xs text-muted-foreground uppercase tracking-wider">Server</p>
			<CopyCommand command={SERVER_COMMAND}>
				{SERVER_COMMAND}
			</CopyCommand>
		</div >
	);
}

function SectionMarker({ number, label }: { number: string; label: string }) {
	return (
		<div className="text-xs text-muted-foreground uppercase tracking-wider">
			{number} / {label}
		</div>
	);
}

function SectionHeader({ title, number }: { title: string; number: string }) {
	return (
		<div className="flex items-center justify-between gap-4">
			<h2 className="font-extrabold text-lg">{title}</h2>
			<SectionMarker number={number} label={title} />
		</div>
	);
}

function LinkCard({
	title,
	description,
	href,
}: {
	title: string;
	description: string;
	href: string;
}) {
	return (
		<a
			href={href}
			target="_blank"
			rel="noreferrer"
			className="block rounded-sm border bg-card p-4 transition-colors hover:border-primary hover:text-primary"
		>
			<div className="flex items-start gap-2">
				<span className="text-primary">[*]</span>
				<div className="space-y-1">
					<h3 className="font-semibold">{title}</h3>
					<p className="text-sm text-muted-foreground">{description}</p>
				</div>
			</div>
		</a>
	);
}

function PluginsSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<SectionHeader title="Plugins" number="03" />
			<LinkCard
				title="Compaction"
				description="Save context by compacting older messages when close to an overflow."
				href={COMPACTION_PLUGIN_URL}
			/>
		</section>
	);
}

function ProvidersSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<SectionHeader title="Providers" number="02" />
			<div className="grid gap-3">
				{PROVIDERS.map((provider) => (
					<a
						key={provider.name}
						href={provider.href}
						target="_blank"
						rel="noreferrer"
						className="block rounded-sm border bg-card p-4 transition-colors hover:border-primary hover:text-primary"
					>
						<div className="flex items-center gap-2">
							<span className="text-primary">[*]</span>
							<h3 className="font-semibold">{provider.name}</h3>
						</div>
					</a>
				))}
			</div>
			<p className="text-sm text-muted-foreground">More providers are coming soon.</p>
		</section>
	);
}

function ClientsSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<SectionHeader title="Clients" number="04" />
			<p className="text-sm text-muted-foreground">Applications that rely on Wingman. If you build one, open up a PR to add it to this section.</p>
			<LinkCard
				title="Web"
				description="A browser client bundled into the Wingman binary."
				href={WEB_CLIENT_URL}
			/>
		</section>
	);
}

function ComingSoonSection() {
	return (
		<section className="px-12 py-8 border-b space-y-2">
			<SectionHeader title="Coming Soon" number="05" />
			<p className="text-sm text-muted-foreground mb-4">Also many more things that aren't listed.</p>
			<div className="grid gap-3 sm:grid-cols-2">
				<div className="rounded-sm border bg-card p-4">
					<h3 className="font-semibold">MCP support</h3>
					<p className="mt-1 text-sm text-muted-foreground">Connect Wingman agents to local or remote Model Context Protocol servers.</p>
				</div>
				<div className="rounded-sm border bg-card p-4">
					<h3 className="font-semibold">Plugin Registry</h3>
					<p className="mt-1 text-sm text-muted-foreground">Discover and install community plugins from a shared registry.</p>
				</div>

				<div className="rounded-sm border bg-card p-4">
					<h3 className="font-semibold">More Providers</h3>
					<p className="mt-1 text-sm text-muted-foreground">At launch Provider support is limited but I'm working on it.</p>
				</div>
			</div>
		</section>
	);
}

function FAQSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<SectionHeader title="FAQ" number="01" />
			<Accordion className="rounded-sm border bg-card px-4">
				{FAQS.map((faq) => (
					<AccordionItem key={faq.question}>
						<AccordionTrigger>{faq.question}</AccordionTrigger>
						<AccordionContent className="text-muted-foreground leading-relaxed">
							{faq.answer}
						</AccordionContent>
					</AccordionItem>
				))}
			</Accordion>
		</section>
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
					{/* <NavLink name="Discord" url={DISCORD_URL} /> */}
				</div>
			</nav>
			<section className="border-b p-12 space-y-8">
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
			<FAQSection />
			<ProvidersSection />
			<PluginsSection />
			<ClientsSection />
			<ComingSoonSection />
			<footer className="px-6 py-4 text-center">
				<p className="text-sm text-muted-foreground font-mono">
					Wingman
				</p>
			</footer>
		</main >
	);
}
