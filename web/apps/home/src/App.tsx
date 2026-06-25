import { useEffect, useState } from "react"
import WingmanIcon from "@wingman/core/assets/WingmanBlue.png"
import { Badge } from "@wingman/core/components/core/badge"
import { buttonVariants } from "@wingman/core/components/core/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@wingman/core/components/core/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table"
import { cn } from "@wingman/core/lib/utils"

const primaryLinks = [
  {
    title: "Wingman",
    href: "https://wingman.actor",
    label: "Site",
    description: "Public home for the open-source client-agnostic agent harness.",
  },
  {
    title: "Docs",
    href: "https://docs.wingman.actor",
    label: "Docs",
    description: "Install, configure, serve, and build against the Wingman HTTP API.",
  },
  {
    title: "GitHub",
    href: "https://github.com/chaserensberger/wingman",
    label: "Source",
    description: "Source code, issues, pull requests, releases, and project history.",
  },
  {
    title: "Linear",
    href: "https://linear.app/wingteam/",
    label: "Backlog",
    description: "Active planning, beta priorities, bugs, and execution tracking.",
  },
] as const

const secondaryLinks = [
  {
    title: "exe.dev",
    href: "https://exe.dev",
    label: "Studio",
    description: "Related work from the same orbit as Wingman.",
  },
  {
    title: "Discord",
    href: "https://discord.gg/Mw4KURek3Q",
    label: "Community",
    description: "Talk through ideas, report rough edges, and follow along.",
  },
  {
    title: "New GitHub Issue",
    href: "https://github.com/chaserensberger/wingman/issues/new",
    label: "Feedback",
    description: "File bugs, request providers, or suggest missing documentation.",
  },
  {
    title: "News",
    href: "https://news.wingman.actor",
    label: "Side quest",
    description: "A small Hacker News client from the Wingman maker.",
  },
] as const

const healthChecks = [
  {
    title: "wingman.actor",
    href: "https://wingman.actor",
  },
  {
    title: "news.wingman.actor",
    href: "https://news.wingman.actor",
  },
] as const

type LinkItem = (typeof primaryLinks | typeof secondaryLinks)[number]
type HealthCheck = (typeof healthChecks)[number]
type HealthStatus = "checking" | "online" | "offline"

function ExternalLinkCard({ link, priority = "normal" }: { link: LinkItem; priority?: "high" | "normal" }) {
  return (
    <a href={link.href} target="_blank" rel="noreferrer" className="group block h-full">
      <Card
        className={cn(
          "h-full rounded-sm transition-colors group-hover:border-primary group-hover:bg-primary/5",
          priority === "high" && "border-primary/30 bg-primary/5"
        )}
      >
        <CardHeader>
          <div className="space-y-2">
            <Badge variant={priority === "high" ? "default" : "outline"}>{link.label}</Badge>
            <CardTitle className="flex items-center gap-2 text-lg">
              {link.title}
              <span aria-hidden="true" className="text-primary transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5">
                -&gt;
              </span>
            </CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <CardDescription>{link.description}</CardDescription>
          <p className="mt-4 break-all text-xs text-muted-foreground/70">{link.href}</p>
        </CardContent>
      </Card>
    </a>
  )
}

function SectionHeader({ markerId, title, markerTitle = title }: { markerId: string; title: string; markerTitle?: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b pb-3">
      <h2 className="text-lg font-extrabold tracking-tight">{title}</h2>
      <div className="shrink-0 text-xs uppercase tracking-wider text-muted-foreground">
        {markerId} / {markerTitle}
      </div>
    </div>
  )
}

function HealthCheckRow({ check }: { check: HealthCheck }) {
  const [status, setStatus] = useState<HealthStatus>("checking")

  useEffect(() => {
    const controller = new AbortController()
    const timeout = window.setTimeout(() => controller.abort(), 6000)

    fetch(check.href, {
      cache: "no-store",
      mode: "no-cors",
      signal: controller.signal,
    })
      .then(() => setStatus("online"))
      .catch(() => setStatus("offline"))
      .finally(() => window.clearTimeout(timeout))

    return () => {
      window.clearTimeout(timeout)
      controller.abort()
    }
  }, [check.href])

  const statusLabel = {
    checking: "Checking",
    online: "Reachable",
    offline: "Unreachable",
  }[status]

  return (
    <TableRow>
      <TableCell className="font-medium">{check.title}</TableCell>
      <TableCell>
        <Badge variant={status === "online" ? "default" : status === "offline" ? "destructive" : "outline"}>
          {statusLabel}
        </Badge>
      </TableCell>
      <TableCell className="text-right">
        <a
          href={check.href}
          target="_blank"
          rel="noreferrer"
          className="text-muted-foreground hover:text-primary hover:underline hover:underline-offset-4"
        >
          Open -&gt;
        </a>
      </TableCell>
    </TableRow>
  )
}

export function App() {
  return (
    <main className="min-h-screen bg-[radial-gradient(circle_at_top_left,var(--color-primary)_0,transparent_28rem)]/10 px-4 py-6 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex min-h-[calc(100vh-3rem)] max-w-6xl flex-col border bg-background/95 shadow-sm">
        <header className="flex flex-col gap-6 border-b px-5 py-6 sm:px-8 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-4">
            <img src={WingmanIcon} alt="Wingman" className="size-14 rounded-sm border bg-card p-1" />
            <div>
              <p className="text-xs uppercase tracking-[0.45em] text-muted-foreground">Home</p>
              <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Wingman links</h1>
            </div>
          </div>
          <a
            href="https://github.com/chaserensberger/wingman"
            target="_blank"
            rel="noreferrer"
            className={cn(buttonVariants({ variant: "outline", size: "lg" }), "w-full sm:w-fit")}
          >
            Open repo -&gt;
          </a>
        </header>

        <section className="space-y-5 px-5 py-8 sm:px-8">
          <SectionHeader markerId="01" title="Core destinations" />
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {primaryLinks.map((link) => (
              <ExternalLinkCard key={link.href} link={link} priority="high" />
            ))}
          </div>
        </section>

        <section className="space-y-5 border-t px-5 py-8 sm:px-8">
          <SectionHeader markerId="02" title="Health checks" />
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>App</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Link</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {healthChecks.map((check) => (
                <HealthCheckRow key={check.href} check={check} />
              ))}
            </TableBody>
          </Table>
          <p className="text-sm text-muted-foreground">More services will be added here later.</p>
        </section>

        <section className="space-y-5 border-t px-5 py-8 sm:px-8">
          <SectionHeader markerId="03" title="Related paths" />
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {secondaryLinks.map((link) => (
              <ExternalLinkCard key={link.href} link={link} />
            ))}
          </div>
        </section>

        <footer className="mt-auto flex flex-col gap-3 border-t px-5 py-5 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:px-8">
          <span>Wingman / home</span>
          <span>Docs, source, planning, and adjacent project links.</span>
        </footer>
      </div>
    </main>
  )
}
