import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { CopyIcon, CheckIcon } from "@phosphor-icons/react";
import { Alert, AlertDescription, AlertTitle } from "@wingman/core/components/core/alert";
import { Button } from "@wingman/core/components/core/button";
import { Card, CardDescription, CardTitle } from "@wingman/core/components/core/card";
import { Markdown } from "@wingman/core/components/core/markdown";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@wingman/core/components/core/tabs";
import { TypographyH2 } from "@wingman/core/components/core/typography";
import WingmanIcon from "../assets/WingmanBlue.png";
import { ASCIILOGO } from "../components/ascii-logo";

export const Route = createFileRoute("/")({
  component: RouteComponent,
});

function RouteComponent() {
  return <Hero />;
}

const SERVER_COMMAND = "curl -fsSL https://wingman.actor/install | bash";
const ENABLE_COMMAND = "wingman service start";
const GITHUB_URL = "https://github.com/chaserensberger/wingman";
const DOCS_URL = "https://docs.wingman.actor";
// const ISSUE_URL = "https://github.com/chaserensberger/wingman/issues/new";
const DISCORD_URL = "https://discord.gg/Mw4KURek3Q";
const COMPACTION_PLUGIN_URL =
  "https://github.com/ChaseRensberger/wingman/blob/main/plugins/compaction/compaction.go";
const WINGMAN_API_EXAMPLE = `\`\`\`bash
wingman api createSession -d '{"title":"Research"}'
\`\`\``;
const HTTP_API_EXAMPLE = `\`\`\`bash
curl -sS -X POST "$WINGMAN_URL/sessions" \\
  -u "$WINGMAN_AUTH" \\
  -H "Content-Type: application/json" \\
  -d '{"title":"Research"}'
\`\`\``;
const TYPESCRIPT_SDK_EXAMPLE = `\`\`\`bash
npm install @wingman-actor/client
\`\`\`

\`\`\`typescript
import { createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: process.env.WINGMAN_URL!,
  password: process.env.WINGMAN_PASSWORD,
});

const session = await client.sessions.create({ title: "Research" });
\`\`\``;
const GO_SDK_EXAMPLE = `\`\`\`bash
go get github.com/chaserensberger/wingman/client
\`\`\`

\`\`\`go
package main

import (
  "context"
  "log"

  "github.com/chaserensberger/wingman/client"
)

func main() {
  ctx := context.Background()
  wingman, err := client.NewLocal(ctx)
  if err != nil {
    log.Fatal(err)
  }

  title := "Research"
  _, err = wingman.CreateSessionWithResponse(
    ctx, nil, client.CreateSessionRequest{Title: &title},
  )
  if err != nil {
    log.Fatal(err)
  }
}
\`\`\``;
const WINGMODELS_EXAMPLE = `

\`\`\`go
import (
  "github.com/chaserensberger/wingman/models"
  provider "github.com/chaserensberger/wingman/models/providers"
  gemini "github.com/chaserensberger/wingman/models/providers/google"
)

msg, err := provider.NewClient(nil).Generate(ctx, models.Request{
  Model: gemini.Model("gemini-3.6-flash"),
  Messages: []models.Message{
    models.NewUserText("Explain Wingman in one sentence."),
  },
})
\`\`\``;
const FEATURES = [
  {
    title: "Client-agnostic runtime",
    description: "Run Wingman as the backend for any client that depends on LLM functionality.",
  },
  {
    title: "Durable run queue",
    description:
      "Each submitted prompt is admitted, ordered, and tracked independently of the request that created it.",
  },
  {
    title: "Per-tool permissions",
    description:
      "Ask before sensitive actions, approve once or remember an exact session-scoped grant.",
  },
  {
    title: "Reconnectable events",
    description:
      "Durable, versioned events replay before live delivery so clients can recover their view of a session.",
  },
  {
    title: "Plugins",
    description:
      "Add tools and session behavior with in-process Go modules or out-of-process JSON-RPC plugins.",
  },
  {
    title: "Provider-agnostic",
    description: "Wingman ships its own provider-agnostic model SDK (WingModels).",
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
    <Card size="sm" className="text-sm flex-row items-center gap-3 px-3 py-3">
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
    </Card>
  );
}

function InstallSection() {
  return (
    <div className="space-y-4">
      <p className="text-xs text-muted-foreground uppercase tracking-wider">INSTALL</p>
      <CopyCommand command={SERVER_COMMAND}>{SERVER_COMMAND}</CopyCommand>
      <p className="text-xs text-muted-foreground font-mono">
        SUPPORTED: Linux (x86_64, ARM64) · macOS (Apple Silicon, Intel)
      </p>

      <p className="text-xs text-muted-foreground uppercase tracking-wider">ENABLE</p>
      <CopyCommand command={ENABLE_COMMAND}>{ENABLE_COMMAND}</CopyCommand>
      <p className="text-xs text-muted-foreground">
        Uses systemd service on Linux and LaunchAgent on macOS.
      </p>
    </div>
  );
}

function SectionMarker({ id, title }: { id: string; title: string }) {
  return (
    <div className="text-xs text-muted-foreground uppercase tracking-wider">
      {id} / {title}
    </div>
  );
}

function SectionHeader({
  title,
  markerId,
  markerTitle = title,
}: {
  title: string;
  markerId: string;
  markerTitle?: string;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <TypographyH2 className="text-lg font-extrabold">{title}</TypographyH2>
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
    <a href={href} target="_blank" rel="noreferrer" className="block">
      <Card
        size="sm"
        className="h-full px-3 transition-[color,border-color] duration-150 hover:border-primary hover:text-primary"
      >
        <div className="flex items-start gap-2">
          <span className="text-primary">[*]</span>
          <div className="space-y-1">
            <CardTitle>{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
      </Card>
    </a>
  );
}

function WhatIsWingmanSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <SectionHeader title="What is Wingman?" markerId="01" markerTitle="Wingman" />
      <p className="text-sm text-muted-foreground">
        Wingman is yet another agent harness but this one is:
      </p>
      <ul className="space-y-3">
        <li className="flex items-start gap-2 text-sm text-muted-foreground">
          <span className="text-primary">[*]</span>
          <span>Written in Go.</span>
        </li>
        <li className="flex items-start gap-2 text-sm text-muted-foreground">
          <span className="text-primary">[*]</span>
          <span>
            Client agnostic - can run multiple clients/UIs on a single machine that all use Wingman
            as a dependency. Wingman is decoupled from any specific use case, so it doesn't come
            bundled with a coding TUI, but you can build a coding TUI on top of it.
          </span>
        </li>
        <li className="flex items-start gap-2 text-sm text-muted-foreground">
          <span className="text-primary">[*]</span>
          <span>
            Independent of external dependencies, making it ideal for running in secure or airgapped
            environments.
          </span>
        </li>
        <li className="flex items-start gap-2 text-sm text-muted-foreground">
          <span className="text-primary">[*]</span>
          <span>
            Highly extensible - plugin support via in-process Go modules or out-of-process JSON-RPC.
            Can register tools, attach to lifecycle events, rewrite history, etc...
          </span>
        </li>
      </ul>
      <a href={DOCS_URL}>
        <Button>Read Docs -&gt;</Button>
      </a>
    </section>
  );
}

function FeaturesSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <SectionHeader title="Features" markerId="02" />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {FEATURES.map((feature) => (
          <Card key={feature.title} size="sm" className="px-3">
            <div className="flex items-start gap-2">
              <span className="text-primary">[*]</span>
              <div className="space-y-1">
                <CardTitle>{feature.title}</CardTitle>
                <CardDescription>{feature.description}</CardDescription>
              </div>
            </div>
          </Card>
        ))}
      </div>
    </section>
  );
}

function ReliableExecutionSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <SectionHeader title="Reliable execution" markerId="03" />
      <p className="max-w-3xl text-sm text-muted-foreground">
        Each prompt gets a durable run. Wingman records its state as it runs. After a restart,
        queued runs continue.
      </p>
      <div className="grid gap-3 lg:grid-cols-[1.35fr_1fr]">
        <Card size="sm" className="gap-0 overflow-hidden p-0 text-xs">
          <div className="flex items-center justify-between border-b px-4 py-3 text-muted-foreground">
            <span>RUN / run_01H...</span>
            <span className="text-primary">COMPLETED</span>
          </div>
          <div className="space-y-3 p-4">
            <ExecutionRow
              state="01"
              title="Queued"
              detail="Prompt admitted with its agent and model snapshot"
            />
            <ExecutionRow
              state="02"
              title="Model attempt"
              detail="anthropic/claude-sonnet-5 · 1,248 tokens"
            />
            <ExecutionRow state="03" title="Tool use" detail="bash · authorized · completed" />
            <ExecutionRow
              state="04"
              title="Settled"
              detail="Run and final message committed together"
              active
            />
          </div>
        </Card>
        <Card size="sm" className="px-3">
          <p className="text-xs uppercase tracking-wider text-muted-foreground">After a restart</p>
          <ul className="space-y-3 text-sm text-muted-foreground">
            <li className="flex gap-2">
              <span className="text-primary">[*]</span>
              <span>Queued runs continue.</span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary">[*]</span>
              <span>Started provider calls and tool uses become interrupted.</span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary">[*]</span>
              <span>Clients reload session state after an event resync.</span>
            </li>
          </ul>
        </Card>
      </div>
    </section>
  );
}

function ExecutionRow({
  state,
  title,
  detail,
  active = false,
}: {
  state: string;
  title: string;
  detail: string;
  active?: boolean;
}) {
  return (
    <div className="flex gap-3">
      <div
        className={`flex size-6 shrink-0 items-center justify-center border text-[0.625rem] ${active ? "border-primary bg-primary/10 text-primary" : "text-muted-foreground"}`}
      >
        {state}
      </div>
      <div className="min-w-0 pt-0.5">
        <p className="font-medium text-foreground">{title}</p>
        <p className="mt-1 text-muted-foreground">{detail}</p>
      </div>
    </div>
  );
}

function PluginsSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <div>
        <SectionHeader title="Plugins" markerId="06" />
        <p className="text-sm text-muted-foreground">
          Extend Wingman with in-process Go modules or out-of-process JSON-RPC plugins.
        </p>
      </div>
      <LinkCard
        title="Compaction"
        description="Save context by compacting older messages when close to a session overflow."
        href={COMPACTION_PLUGIN_URL}
      />
    </section>
  );
}

function ProvidersSection() {
  return (
    <section className="px-6 py-8 border-b space-y-2 sm:px-12">
      <SectionHeader
        title="Multi-provider support via WingModels"
        markerId="04"
        markerTitle="WingModels"
      />
      <p className="text-sm text-muted-foreground">
        Wingman ships its own provider-agnostic model SDK (written in Go). One typed request,
        response, event, and tool language; provider quirks live in adapters.
      </p>
      <Markdown>{WINGMODELS_EXAMPLE}</Markdown>
      <div className="space-y-3">
        <p className="text-xs text-muted-foreground uppercase tracking-wider">
          Supported Providers
        </p>
        <ul className="space-y-3">
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>OpenAI (Can use Codex/GPT Pro subscription)</span>
          </li>
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>Anthropic</span>
          </li>
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>OpenCode Zen/Go</span>
          </li>
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>Gemini</span>
          </li>
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>OpenRouter</span>
          </li>
          <li className="flex items-start gap-2 text-sm text-muted-foreground">
            <span className="text-primary">[*]</span>
            <span>DeepSeek</span>
          </li>
        </ul>

        <p className="text-sm text-muted-foreground">
          I tend to only support the most recent models. This will probably change in the future. If
          you want a new provider/model that is not available, create an issue on GitHub.
        </p>
      </div>
    </section>
  );
}

function ConnectSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <SectionHeader title="Connect to Wingman" markerId="05" />
      <Tabs defaultValue="wingman-cli">
        <TabsList>
          <TabsTrigger value="wingman-cli">Wingman CLI</TabsTrigger>
          <TabsTrigger value="http">HTTP</TabsTrigger>
          <TabsTrigger value="typescript">TypeScript</TabsTrigger>
          <TabsTrigger value="go">Go</TabsTrigger>
        </TabsList>
        <TabsContent value="wingman-cli">
          <div className="space-y-4">
            <Markdown>{WINGMAN_API_EXAMPLE}</Markdown>
            <a
              href={`${DOCS_URL}/reference/cli/#api-command`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              Read the docs -&gt;
            </a>
          </div>
        </TabsContent>
        <TabsContent value="http">
          <div className="space-y-4">
            <Markdown>{HTTP_API_EXAMPLE}</Markdown>
            <a
              href={`${DOCS_URL}/build-clients/http-api-basics`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              Read the docs -&gt;
            </a>
          </div>
        </TabsContent>
        <TabsContent value="typescript">
          <div className="space-y-4">
            <Markdown>{TYPESCRIPT_SDK_EXAMPLE}</Markdown>
            <a
              href={`https://www.npmjs.com/package/@wingman-actor/client`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              NPM Package -&gt;
            </a>
            <br />
            <a
              href={`${DOCS_URL}/build-clients/typescript-sdk`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              Read the docs -&gt;
            </a>
          </div>
        </TabsContent>
        <TabsContent value="go">
          <div className="space-y-4">
            <Markdown>{GO_SDK_EXAMPLE}</Markdown>

            <a
              href={`https://pkg.go.dev/github.com/chaserensberger/wingman/client`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              Go Package -&gt;
            </a>
            <br />
            <a
              href={`${DOCS_URL}/build-clients/go-sdk`}
              className="text-sm text-primary underline-offset-4 hover:underline"
            >
              Read the docs -&gt;
            </a>
          </div>
        </TabsContent>
      </Tabs>
    </section>
  );
}

function ClientsSection() {
  return (
    <section className="px-6 py-8 border-b space-y-4 sm:px-12">
      <div>
        <SectionHeader title="Clients" markerId="07" />
        <p className="text-sm text-muted-foreground">
          Applications that rely on Wingman. If you build one, open up a PR to add it to this
          section.
        </p>
      </div>
      <Card size="sm" className="px-3">
        <div className="flex items-start gap-2">
          <span className="text-primary">[*]</span>
          <CardTitle>Console - Ships with the Wingman binary</CardTitle>
        </div>
        <div className="space-y-2">
          <img
            className="w-full rounded-sm border bg-background"
            src="/wingman-console.png"
            alt="Wingman console session showing a technical explanation and message composer"
          />
        </div>
      </Card>
    </section>
  );
}

/* function OperationsSection() {
	return (
		<section className="px-6 py-8 border-b space-y-4 sm:px-12">
			<SectionHeader title="Observability" markerId="07" />
			<p className="max-w-3xl text-sm text-muted-foreground">
				Use the daemon API to see where work is waiting, why an event client disconnected, and whether an external plugin is healthy.
			</p>
			<div className="overflow-hidden border bg-card font-mono text-xs">
				<div className="border-b px-4 py-3 text-muted-foreground">OPERATOR SURFACES</div>
				<div className="grid divide-y sm:grid-cols-3 sm:divide-x sm:divide-y-0">
					<ObservabilityItem endpoint="/diagnostics" question="Is the daemon under pressure?" detail="Queued and active runs, execution scopes, event subscriber backlog, and aggregate plugin health." />
					<ObservabilityItem endpoint="/plugins" question="Which plugin failed?" detail="Plugin status, process details, and recent diagnostics for each external plugin." />
					<ObservabilityItem endpoint="/logs" question="What happened recently?" detail="A bounded buffer of recent daemon log entries for local diagnosis." />
				</div>
			</div>
		</section>
	)
}

function ObservabilityItem({ endpoint, question, detail }: { endpoint: string; question: string; detail: string }) {
	return (
		<div className="space-y-3 p-4">
			<p className="text-primary">GET {endpoint}</p>
			<p className="font-sans font-semibold text-foreground">{question}</p>
			<p className="font-sans text-sm text-muted-foreground">{detail}</p>
		</div>
	)
}

function ComingSoonSection() {
	return (
		<section className="px-6 py-8 border-b space-y-4 sm:px-12">
			<div>
				<SectionHeader title="Coming Soon" markerId="08" />
				<p className="text-sm text-muted-foreground mb-4">Also many more things that aren't listed.</p>
			</div>
			<div className="grid auto-rows-fr gap-3 sm:grid-cols-2">
				<div className="h-full rounded-sm border bg-card p-4">
					<h3 className="font-semibold">Plugin Registry</h3>
					<p className="mt-1 text-sm text-muted-foreground">Discover and install community plugins from a shared registry.</p>
				</div>

				<div className="h-full rounded-sm border bg-card p-4">
					<h3 className="font-semibold">TypeScript SDK/OpenAPI Spec</h3>
					<p className="mt-1 text-sm text-muted-foreground">I want to make sure you can use Wingman in any language you would like.</p>
				</div>

				<div className="h-full rounded-sm border bg-card p-4">
					<h3 className="font-semibold">Scheduling</h3>
					<p className="mt-1 text-sm text-muted-foreground">Add Wingman agents to a trigger schedule for things like automatic triage.</p>
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
} */

function NavLink(navItem: { name: string; url: string }) {
  return (
    <a
      href={navItem.url}
      className="text-muted-foreground hover:text-foreground transition-colors hover:underline hover:underline-offset-4"
    >
      {navItem.name}
    </a>
  );
}

function Hero() {
  return (
    <main className="min-h-screen flex flex-col md:max-w-4xl lg:max-w-5xl mx-auto border">
      <nav className="sticky top-0 z-50 flex w-full items-center justify-between border-b bg-background/80 px-6 py-2 backdrop-blur-sm">
        <img src={WingmanIcon} className="w-12 h-12" />
        <div className="flex items-center gap-6">
          <NavLink name="GitHub" url={GITHUB_URL} />
          <NavLink name="Docs" url={DOCS_URL} />
          <NavLink name="Discord" url={DISCORD_URL} />
        </div>
      </nav>
      <Alert className="rounded-none border-x-0 border-t-0 bg-primary/15 px-6 text-primary sm:px-12">
        <AlertTitle className="line-clamp-none">Wingman is not production ready.</AlertTitle>
        <AlertDescription className="text-primary">
          Expect frequent changes to APIs and data models for the time being.
        </AlertDescription>
      </Alert>
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
      <ReliableExecutionSection />
      <ConnectSection />
      <ProvidersSection />
      <PluginsSection />
      <ClientsSection />
      <footer className="px-6 py-4 flex justify-between items-center">
        <p className="text-sm text-muted-foreground font-mono mx-auto">Wingman</p>
        {/*
				<p className='text-sm text-muted-foreground font-mono'>
					Also I made<a href='https://news.wingman.actor'><Button variant="link">a hackernews client</Button></a>
				</p>
				*/}
      </footer>
    </main>
  );
}
