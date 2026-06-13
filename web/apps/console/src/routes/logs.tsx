import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { ArrowClockwiseIcon } from "@phosphor-icons/react";
import { Badge } from "@wingman/core/components/core/badge";
import { Button } from "@wingman/core/components/core/button";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { Input } from "@wingman/core/components/core/input";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { wfetch } from "@/lib/client";
import type { LogEntry } from "@/lib/types";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/logs")({
	component: LogsPage,
});

const levels = ["all", "DEBUG", "INFO", "WARN", "ERROR"] as const;

function LogsPage() {
	const [logs, setLogs] = useState<LogEntry[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [level, setLevel] = useState<(typeof levels)[number]>("all");
	const [filter, setFilter] = useState("");

	async function load() {
		try {
			const data = (await wfetch("/logs")) as LogEntry[];
			setLogs(data);
			setError("");
		} catch (err) {
			setError(String(err));
		} finally {
			setLoading(false);
		}
	}

	useEffect(() => {
		let cancelled = false;
		async function tick() {
			if (!cancelled) await load();
		}
		tick();
		const id = window.setInterval(tick, 2000);
		return () => {
			cancelled = true;
			window.clearInterval(id);
		};
	}, []);

	const filtered = logs.filter((log) => {
		if (level !== "all" && log.level !== level) return false;
		const haystack = `${log.time || ""} ${log.level || ""} ${log.msg || ""} ${log.raw}`.toLowerCase();
		return haystack.includes(filter.toLowerCase());
	});

	return (
		<div className="mx-auto max-w-7xl px-4 py-6">
			<div className="mb-4">
				<PageBreadcrumb items={[{ label: "Logs" }]} />
				<div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<div>
						<h1 className="text-lg font-semibold">Current logs</h1>
						<p className="text-sm text-muted-foreground">Recent logs from this Wingman process.</p>
					</div>
					<Button variant="outline" size="sm" onClick={load} disabled={loading}>
						<ArrowClockwiseIcon className={cn("size-4", loading && "animate-spin")} />
						Refresh
					</Button>
				</div>
			</div>

			<Card size="sm" className="gap-4">
				<CardHeader>
					<CardTitle>Log stream</CardTitle>
					<div className="flex flex-wrap items-center gap-2">
						<Input
							value={filter}
							onChange={(event) => setFilter(event.target.value)}
							placeholder="Filter logs..."
							className="h-8 w-52"
						/>
						<div className="flex rounded-lg border bg-muted/45 p-1">
							{levels.map((item) => (
								<Button
									key={item}
									type="button"
									onClick={() => setLevel(item)}
									variant="ghost"
									size="xs"
									className={cn(
										"h-auto rounded-md px-2 py-1 text-xs font-medium uppercase",
										level === item ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
									)}
								>
									{item.toLowerCase()}
								</Button>
							))}
						</div>
					</div>
				</CardHeader>
				<CardContent>
					{error ? <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</div> : null}
					<div className="overflow-hidden rounded-lg border bg-background">
						{filtered.length === 0 ? (
							<div className="p-6 text-sm text-muted-foreground">{loading ? "Loading logs..." : "No logs match the current filters."}</div>
						) : (
							<div className="max-h-[70vh] overflow-auto">
								{filtered.map((log, index) => (
									<LogRow key={`${log.time || "raw"}-${index}`} log={log} />
								))}
							</div>
						)}
					</div>
				</CardContent>
			</Card>
		</div>
	);
}

function LogRow({ log }: { log: LogEntry }) {
	const attrs = log.attrs ? JSON.stringify(log.attrs, null, 2) : "";
	return (
		<div className="grid gap-2 border-b p-3 last:border-b-0 lg:grid-cols-[12rem_5rem_1fr]">
			<div className="text-xs text-muted-foreground">{formatTime(log.time)}</div>
			<div>
				<Badge variant={levelVariant(log.level)} className="font-mono">
					{log.level || "raw"}
				</Badge>
			</div>
			<div className="min-w-0 space-y-2">
				<div className="break-words text-sm">{log.msg || log.raw}</div>
				{attrs ? <pre className="overflow-x-auto rounded-md bg-muted/50 p-2 text-xs text-muted-foreground">{attrs}</pre> : null}
			</div>
		</div>
	);
}

function formatTime(value?: string) {
	if (!value) return "";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return value;
	return date.toLocaleTimeString([], { hour12: false }) + "." + String(date.getMilliseconds()).padStart(3, "0");
}

function levelVariant(level?: string) {
	switch (level) {
		case "ERROR":
			return "destructive" as const;
		case "WARN":
			return "secondary" as const;
		case "DEBUG":
			return "outline" as const;
		default:
			return "ghost" as const;
	}
}
