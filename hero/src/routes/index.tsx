import { createFileRoute } from '@tanstack/react-router'
import { useState } from "react";
import { CopyIcon, CheckIcon } from "@phosphor-icons/react";
import { Button } from "@/components/core/button";
import { Markdown } from "@/components/core/markdown";
import WingmanIcon from "../assets/WingmanBlue.png";
import { ASCIILOGO } from '../components/ascii-logo';

export const Route = createFileRoute('/')({
	component: RouteComponent,
})

function RouteComponent() {
	return <Hero />
}

const SERVER_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const ENABLE_COMMAND = "sudo wingman up";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";
const DOCS_URL = "https://docs.wingman.actor";
const ISSUE_URL = "https://github.com/chaserensberger/wingman/issues/new";
// const DISCORD_URL = "";
const COMPACTION_PLUGIN_URL = "https://github.com/ChaseRensberger/wingman/blob/main/plugins/compaction/compaction.go";
const WINGMODELS_EXAMPLE = `

\`\`\`go
func main() {
  ref, ok := models.ParseModelRef("openai/gpt-5.5")
  if !ok {
    log.Fatal("invalid model ref")
  }

  client := provider.NewClient(nil)

  msg, err := client.Generate(context.Background(), models.Request{
    Model: ref,
    System: "You are concise.",
    Messages: []models.Message{
      models.NewUserText("Explain Wingman in one sentence."),
    },
    Generation: models.Generation{MaxTokens: 80},
  })
  if err != nil {
    log.Fatal(err)
  }

  for _, part := range msg.Content {
    if text, ok := part.(models.TextPart); ok {
      fmt.Println(text.Text)
    }
  }
}
\`\`\``;

const FEATURES = [
	{
		title: "Client-agnostic runtime",
		description:
			"Run Wingman as the backend for any client that depends on LLM functionality.",
	},
	{
		title: "Extendable",
		description:
			"Strong plugin support so you can extend session behavior however you want.",
	},
	{
		title: "Provider-agnostic",
		description:
			"Wingman ships its own provider-agnostic model SDK (WingModels).",
	},
	{
		title: "Context handoff",
		description:
			"Swap between provider/model combinations with minimal (and often zero) data loss."
	},
	{
		title: "Bring your own storage",
		description:
			"Wingman ships with a default sqlite3 adapter but the storage provider is also agnostic."
	},
	{
		title: "HTTP API",
		description:
			"Communicate with Wingman via HTTP. Stdio and other protocols coming later."
	}
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
		<div className="space-y-4">
			<p className="text-xs text-muted-foreground uppercase tracking-wider">INSTALL</p>
			<CopyCommand command={SERVER_COMMAND}>
				{SERVER_COMMAND}
			</CopyCommand>

			<p className="text-xs text-muted-foreground uppercase tracking-wider">ENABLE</p>
			<CopyCommand command={ENABLE_COMMAND}>
				{ENABLE_COMMAND}
			</CopyCommand>
		</div >
	);
}

function SectionMarker({ id, title }: { id: string; title: string }) {
	return (
		<div className="text-xs text-muted-foreground uppercase tracking-wider">
			{id} / {title}
		</div>
	);
}

function SectionHeader({ title, markerId, markerTitle = title }: { title: string; markerId: string; markerTitle?: string }) {
	return (
		<div className="flex items-center justify-between gap-4">
			<h2 className="font-extrabold text-lg">{title}</h2>
			<SectionMarker id={markerId} title={markerTitle} />
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

function WhatIsWingmanSection() {
	return (
		<section className="px-6 py-8 border-b space-y-4 sm:px-12">
			<SectionHeader title="What is Wingman?" markerId="01" markerTitle="Wingman" />
			<p className='text-sm text-muted-foreground'>Wingman is yet another agent harness but this one is:</p>
			<ul className="space-y-3">
				<li className="flex items-start gap-2 text-sm text-muted-foreground">
					<span className="text-primary">[*]</span>
					<span>Written in Go.</span>
				</li>
				<li className="flex items-start gap-2 text-sm text-muted-foreground">
					<span className="text-primary">[*]</span>
					<span>Client agnostic - can run multiple clients/UIs on a single machine that all use Wingman as a dependency. Wingman is decoupled from any specific use case, so it doesn't come bundled with a coding TUI, but you can run a coding TUI on top of it.</span>
				</li>
				<li className="flex items-start gap-2 text-sm text-muted-foreground">
					<span className="text-primary">[*]</span>
					<span>Doesn't rely on your usual harness dependencies. No Vercel AI SDK, no models.dev, etc...making it ideal for running in secure or airgapped environments.</span>
				</li>
				<li className="flex items-start gap-2 text-sm text-muted-foreground">
					<span className="text-primary">[*]</span>
					<span>Highly extensible - plugin support via in-process Go modules or out-of-process JSON-RPC. Can register tools, attach to lifecycle events, rewrite history, etc...</span>
				</li>
			</ul>
			<a
				href={DOCS_URL}
			>
				<Button>
					Read Docs -&gt;
				</Button>
			</a>
		</section >
	);
}

function FeaturesSection() {
	return (
		<section className="px-6 py-8 border-b space-y-4 sm:px-12">
			<SectionHeader title="Features" markerId="02" />
			<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{FEATURES.map((feature) => (
					<div key={feature.title} className="rounded-sm border bg-card p-4">
						<div className="flex items-start gap-2">
							<span className="text-primary">[*]</span>
							<div className="space-y-1">
								<h3 className="font-semibold">{feature.title}</h3>
								<p className="text-sm text-muted-foreground">{feature.description}</p>
							</div>
						</div>
					</div>
				))}
			</div>
		</section>
	);
}

function PluginsSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<div>
				<SectionHeader title="Plugins" markerId="05" />
				<p className='text-sm text-muted-foreground'>Extend Wingman via in-process Go modules or out-of-process JSON-RPC. If you build one, open up a PR to add it to this section.</p>
			</div>
			<LinkCard
				title="Compaction"
				description="Save context by compacting older messages when close to a session overflow."
				href={COMPACTION_PLUGIN_URL}
			/>
			<Button disabled>Plugin Registry Coming Soon -&gt;</Button>
		</section>
	);
}

function ProvidersSection() {
	return (
		<section className="px-12 py-8 border-b space-y-2">
			<SectionHeader title="Multi-provider support via WingModels" markerId="04" markerTitle='WingModels' />
			<p className='text-sm text-muted-foreground'>
				Wingman ships its own provider-agnostic model SDK (written in Go). One typed request, response, event, and tool language; provider quirks live in adapters, not in calling code.
			</p>
			<Markdown>{WINGMODELS_EXAMPLE}</Markdown>
			<div className="space-y-3">
				<p className="text-xs text-muted-foreground uppercase tracking-wider">Supported Providers (More coming soon)</p>
				<ul className="space-y-3">
					<li className="flex items-start gap-2 text-sm text-muted-foreground">
						<span className="text-primary">[*]</span>
						<span>OpenAI</span>
					</li>
					<li className="flex items-start gap-2 text-sm text-muted-foreground">
						<span className="text-primary">[*]</span>
						<span>Anthropic</span>
					</li>
					<li className="flex items-start gap-2 text-sm text-muted-foreground">
						<span className="text-primary">[*]</span>
						<span>OpenCode Zen</span>
					</li>
				</ul>
			</div>
		</section>
	);
}

function ClientsSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<div>
				<SectionHeader title="Clients" markerId="06" />
				<p className="text-sm text-muted-foreground">Applications that rely on Wingman. If you build one, open up a PR to add it to this section.</p>
			</div>
			<div className="rounded-sm border bg-card p-4 space-y-4">
				<div className="flex items-start gap-2">
					<span className="text-primary">[*]</span>
					<h3 className="font-semibold">Web</h3>
				</div>
				<div className="space-y-2">
					<video
						className="w-full rounded-sm border bg-background"
						src="/wingman-web-demo-trimmed.mp4"
						autoPlay
						muted
						loop
						playsInline
						controls
					/>
				</div>
			</div>
		</section>
	);
}

function ComingSoonSection() {
	return (
		<section className="px-12 py-8 border-b space-y-4">
			<div>
				<SectionHeader title="Coming Soon" markerId="07" />
				<p className="text-sm text-muted-foreground mb-4">Also many more things that aren't listed.</p>
			</div>
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
			<div className='flex items-center gap-2'>
				<span>Something missing?</span>
				<a href={ISSUE_URL} target="_blank" rel="noreferrer" className="inline-block">
					<Button>Open An Issue -&gt;</Button>
				</a>
			</div>
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
			<div className="border-b bg-primary/10 px-6 py-3 text-sm text-primary sm:px-12">
				<strong>Wingman is not production ready.</strong> Expect frequent changes to apis and data models as I receive feedback and iterate over the next couple weeks.
			</div>
			<section className="border-b p-12 py-24 space-y-8">
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
			<WhatIsWingmanSection />
			<FeaturesSection />
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
