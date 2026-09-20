import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Trigger, TriggerConfig, TriggerOccurrence } from "@wingman-actor/client";
import {
  ArrowUpRightIcon,
  ClockIcon,
  DotsThreeIcon,
  MagnifyingGlassIcon,
  PencilSimpleIcon,
  PlayIcon,
  PlusIcon,
  TimerIcon,
  TrashIcon,
} from "@phosphor-icons/react";
import { Badge } from "@wingman/core/components/core/badge";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";
import { Switch } from "@wingman/core/components/core/switch";
import { Spinner } from "@wingman/core/components/core/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@wingman/core/components/core/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@wingman/core/components/core/dialog";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@wingman/core/components/core/sheet";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { TriggerEditor } from "@/components/trigger-editor";
import { client } from "@/lib/client";
import {
  agentsQuery,
  queryKeys,
  triggerOccurrencesQuery,
  triggersQuery,
  workspacesQuery,
} from "@/lib/queries";
import { showErrorToast, toastManager } from "@/lib/toast";
import { cn, timeAgo } from "@/lib/utils";
import {
  describeSchedule,
  newTriggerConfig,
  occurrenceDuration,
  scheduledTime,
  triggerTemplates,
} from "@/lib/triggers";

export const Route = createFileRoute("/triggers")({ component: TriggersPage });

function OccurrenceStatus({ occurrence }: { occurrence?: TriggerOccurrence }) {
  if (!occurrence) return <span className="text-xs text-muted-foreground">Not run yet</span>;
  const status = occurrence.status;
  const color =
    status === "completed"
      ? "text-success"
      : status === "failed"
        ? "text-destructive"
        : status === "running" || status === "queued"
          ? "text-primary"
          : "text-muted-foreground";
  return (
    <span className={cn("inline-flex items-center gap-2 text-xs", color)}>
      <span className="size-1.5 shrink-0 rounded-full bg-current" />
      {status === "completed"
        ? "Completed"
        : status === "deleted"
          ? "Session deleted"
          : status.charAt(0).toUpperCase() + status.slice(1)}
    </span>
  );
}

function SessionLink({ occurrence }: { occurrence: TriggerOccurrence }) {
  if (!occurrence.session_id || occurrence.status === "deleted") return null;
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="size-11 sm:size-8"
      nativeButton={false}
      render={<Link to="/sessions/$sessionId" params={{ sessionId: occurrence.session_id }} />}
      aria-label={`Open Session for ${occurrence.trigger_name}`}
      title="Open Session"
    >
      <ArrowUpRightIcon className="size-4" />
    </Button>
  );
}

function Activity({
  items,
  loading,
  error,
  compact = false,
}: {
  items: TriggerOccurrence[];
  loading: boolean;
  error: Error | null;
  compact?: boolean;
}) {
  if (loading)
    return (
      <div className="flex items-center gap-2 p-5 text-sm text-muted-foreground">
        <Spinner size={16} /> Loading activity…
      </div>
    );
  if (error)
    return (
      <p role="alert" className="p-5 text-sm text-destructive">
        {error.message}
      </p>
    );
  if (!items.length)
    return (
      <div className="p-8 text-center">
        <ClockIcon className="mx-auto mb-3 size-6 text-muted-foreground" />
        <p className="text-sm">No activity yet</p>
        <p className="mt-2 text-xs text-muted-foreground">
          Scheduled and manual executions will appear here.
        </p>
      </div>
    );
  if (compact)
    return (
      <div className="divide-y">
        {items.map((o) => (
          <div key={o.id} className="px-5 py-4">
            <div className="flex items-center justify-between gap-3">
              <OccurrenceStatus occurrence={o} />
              <SessionLink occurrence={o} />
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {scheduledTime(o.created_at)} · {o.source === "manual" ? "Manual" : "Cron"} ·{" "}
              {occurrenceDuration(o)}
            </div>
            {o.reason && (
              <p className="mt-2 break-words text-xs text-muted-foreground">{o.reason}</p>
            )}
          </div>
        ))}
      </div>
    );
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>When</TableHead>
          <TableHead>Trigger</TableHead>
          <TableHead className="hidden sm:table-cell">Source</TableHead>
          <TableHead className="hidden md:table-cell">Duration</TableHead>
          <TableHead>Result</TableHead>
          <TableHead>
            <span className="sr-only">Session</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((o) => (
          <TableRow key={o.id}>
            <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
              <time dateTime={o.created_at} title={scheduledTime(o.created_at)}>
                {timeAgo(o.created_at)}
              </time>
            </TableCell>
            <TableCell>
              <div className="max-w-52 truncate text-sm" title={o.trigger_name}>
                {o.trigger_name}
              </div>
            </TableCell>
            <TableCell className="hidden sm:table-cell">
              <Badge variant="outline">{o.source === "manual" ? "Manual" : "Cron"}</Badge>
            </TableCell>
            <TableCell className="hidden text-xs text-muted-foreground md:table-cell">
              {occurrenceDuration(o)}
            </TableCell>
            <TableCell className="max-w-sm whitespace-normal">
              <OccurrenceStatus occurrence={o} />
              {o.reason && (
                <p className="mt-1 break-words text-xs text-muted-foreground">{o.reason}</p>
              )}
            </TableCell>
            <TableCell>
              <SessionLink occurrence={o} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function TriggerDetails({
  trigger,
  agentName,
  onClose,
  onEdit,
}: {
  trigger: Trigger;
  agentName: string;
  onClose: () => void;
  onEdit: () => void;
}) {
  const history = useQuery(triggerOccurrencesQuery(trigger.id));
  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent className="w-full gap-0 overflow-y-auto p-0 sm:max-w-xl">
        <SheetHeader className="border-b p-5 pr-12">
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="outline">Cron</Badge>
            <Badge variant={trigger.enabled ? "secondary" : "outline"}>
              {trigger.enabled ? "Enabled" : "Paused"}
            </Badge>
          </div>
          <SheetTitle className="break-words">{trigger.name}</SheetTitle>
          <SheetDescription>
            {describeSchedule(trigger.source.expression)} · {trigger.source.time_zone}
          </SheetDescription>
        </SheetHeader>
        <div className="grid gap-5 border-b p-5">
          <dl className="grid grid-cols-2 gap-4 text-xs">
            <div>
              <dt className="text-muted-foreground">Agent</dt>
              <dd className="mt-1 text-sm">{agentName}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Sessions created</dt>
              <dd className="mt-1 text-sm">{trigger.run_count}</dd>
            </div>
            <div className="col-span-2">
              <dt className="text-muted-foreground">Next scheduled time</dt>
              <dd className="mt-1 text-sm">
                {trigger.next_fire_at
                  ? scheduledTime(trigger.next_fire_at, trigger.source.time_zone)
                  : "Paused"}
              </dd>
            </div>
            <div className="col-span-2">
              <dt className="text-muted-foreground">Expression</dt>
              <dd className="mt-1 text-primary">{trigger.source.expression}</dd>
            </div>
            {trigger.target.working_directory && (
              <div className="col-span-2">
                <dt className="text-muted-foreground">Working directory</dt>
                <dd className="mt-1 break-all">{trigger.target.working_directory}</dd>
              </div>
            )}
          </dl>
          <div>
            <h3 className="mb-2 text-xs text-muted-foreground">Prompt</h3>
            <p className="whitespace-pre-wrap break-words rounded-md border bg-muted/20 p-3 text-sm leading-relaxed">
              {trigger.prompt}
            </p>
          </div>
          <Button variant="outline" onClick={onEdit} className="h-11 sm:h-9">
            <PencilSimpleIcon /> Edit trigger
          </Button>
        </div>
        <div className="flex items-center justify-between border-b px-5 py-4">
          <h3 className="text-sm font-medium">Activity</h3>
          <span className="text-xs text-muted-foreground">Latest 50 occurrences</span>
        </div>
        <Activity
          items={history.data ?? []}
          loading={history.isPending}
          error={history.error}
          compact
        />
      </SheetContent>
    </Sheet>
  );
}

function TriggersPage() {
  const queryClient = useQueryClient();
  const result = useQuery(triggersQuery);
  const history = useQuery(triggerOccurrencesQuery());
  const agents = useQuery(agentsQuery);
  const workspaces = useQuery(workspacesQuery);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | "enabled" | "paused">("all");
  const [editor, setEditor] = useState<{ trigger?: Trigger; initial?: TriggerConfig }>();
  const [selectedID, setSelectedID] = useState<string>();
  const [deleting, setDeleting] = useState<Trigger>();
  const triggers = result.data ?? [];
  const selected = triggers.find((item) => item.id === selectedID);
  const agentName = (id: string) => agents.data?.find((agent) => agent.id === id)?.name ?? id;
  const workspaceName = (item: Trigger) =>
    item.target.workspace_id
      ? (workspaces.data?.find((workspace) => workspace.id === item.target.workspace_id)?.name ??
        "Workspace unavailable")
      : item.target.working_directory || "No workspace";
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.triggers }),
      queryClient.invalidateQueries({ queryKey: ["trigger-occurrences"] }),
      queryClient.invalidateQueries({ queryKey: queryKeys.sessions }),
    ]);
  };
  const state = useMutation({
    mutationFn: (item: Trigger) =>
      client.triggers.setState(item.id, { enabled: !item.enabled, expected_version: item.version }),
    onSuccess: refresh,
    onError: (error) => {
      showErrorToast(error);
      void refresh();
    },
  });
  const fire = useMutation({
    mutationFn: ({ item, requestID }: { item: Trigger; requestID: string }) =>
      client.triggers.fire(item.id, requestID),
    onSuccess: (occurrence) => {
      toastManager.add({
        title:
          occurrence.status === "skipped"
            ? "Execution skipped"
            : occurrence.status === "failed"
              ? "Could not start the run"
              : "Run submitted",
        description: occurrence.reason || "A fresh Session appears in activity below.",
        type: occurrence.status === "failed" ? "destructive" : "info",
      });
      void refresh();
    },
    onError: (error) => {
      showErrorToast(error);
      void refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (item: Trigger) => client.triggers.delete(item.id, item.version),
    onSuccess: async () => {
      setDeleting(undefined);
      await refresh();
    },
    onError: (error) => showErrorToast(error),
  });
  const filtered = triggers.filter(
    (item) =>
      (filter === "all" || item.enabled === (filter === "enabled")) &&
      `${item.name} ${item.prompt} ${agentName(item.target.agent_id)} ${workspaceName(item)}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  const enabled = triggers.filter((item) => item.enabled);
  const next = enabled
    .filter((item) => item.next_fire_at)
    .sort((a, b) => Date.parse(a.next_fire_at!) - Date.parse(b.next_fire_at!))[0];
  const edit = (item: Trigger) => {
    setSelectedID(undefined);
    setEditor({ trigger: item });
  };

  function actions(item: Trigger) {
    const firing = fire.isPending && fire.variables?.item.id === item.id;
    return (
      <div className="flex items-center justify-end gap-1">
        <Button
          variant="outline"
          size="icon-sm"
          className="size-11 sm:size-8"
          disabled={firing}
          aria-label={`Run ${item.name} now`}
          title="Run now"
          onClick={() => fire.mutate({ item, requestID: crypto.randomUUID() })}
        >
          {firing ? <Spinner size={14} /> : <PlayIcon className="size-4" />}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                className="size-11 sm:size-8"
                aria-label={`Actions for ${item.name}`}
              />
            }
          >
            <DotsThreeIcon className="size-5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem onClick={() => setSelectedID(item.id)}>
              <ClockIcon /> View activity
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => edit(item)}>
              <PencilSimpleIcon /> Edit trigger
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onClick={() => setDeleting(item)}>
              <TrashIcon /> Delete trigger
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    );
  }

  function toggle(item: Trigger) {
    return (
      <div className="flex min-h-11 items-center sm:min-h-8">
        <Switch
          checked={item.enabled}
          disabled={state.isPending && state.variables?.id === item.id}
          onCheckedChange={() => state.mutate(item)}
          aria-label={`${item.enabled ? "Pause" : "Enable"} ${item.name}`}
        />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6">
      <PageBreadcrumb items={[{ label: "Triggers" }]} />
      <div className="mb-6 mt-5 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Work on your schedule.</h1>
          <p className="mt-2 max-w-xl text-sm text-muted-foreground">
            Give an Agent a task. Choose when it starts. Come back to the results.
          </p>
        </div>
        <Button onClick={() => setEditor({})} className="h-11 sm:h-9">
          <PlusIcon className="size-4" /> New trigger
        </Button>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[
          {
            label: "Triggers",
            value: result.data ? String(triggers.length) : "—",
            note: "Managed by this Client",
            className: "",
          },
          {
            label: "Enabled",
            value: result.data ? String(enabled.length) : "—",
            note: `${triggers.length - enabled.length} paused`,
            className: "text-success",
          },
          {
            label: "Runs submitted",
            value: result.data
              ? triggers.reduce((total, item) => total + item.run_count, 0).toLocaleString()
              : "—",
            note: "Across current triggers",
            className: "",
          },
          {
            label: "Next scheduled",
            value: next ? scheduledTime(next.next_fire_at!, next.source.time_zone) : "—",
            note: next ? `${next.name} · ${next.source.time_zone}` : "No enabled schedule",
            className: "text-primary text-base!",
          },
        ].map((stat) => (
          <div key={stat.label} className="min-w-0 rounded-md border bg-card p-4">
            <p className="text-xs text-muted-foreground">{stat.label}</p>
            <p className={cn("mt-3 truncate text-2xl font-medium", stat.className)}>{stat.value}</p>
            <p className="mt-2 truncate text-xs text-muted-foreground" title={stat.note}>
              {stat.note}
            </p>
          </div>
        ))}
      </div>

      {result.error ? (
        <div role="alert" className="rounded-md border border-destructive/30 p-5">
          <p className="text-sm text-destructive">{result.error.message}</p>
          <Button variant="outline" className="mt-3" onClick={() => void result.refetch()}>
            Try again
          </Button>
        </div>
      ) : result.isPending ? (
        <div className="flex items-center gap-2 py-12 text-sm text-muted-foreground">
          <Spinner size={20} /> Loading triggers…
        </div>
      ) : triggers.length === 0 ? (
        <div className="rounded-md border bg-card">
          <div className="border-b px-6 py-10 text-center">
            <TimerIcon className="mx-auto mb-4 size-8 text-primary" />
            <h2 className="text-lg font-medium">Set the next thing in motion.</h2>
            <p className="mx-auto mt-3 max-w-lg text-sm leading-relaxed text-muted-foreground">
              A trigger starts a fresh Session with your instructions. Use a starting point below,
              or create your own task.
            </p>
          </div>
          <div className="grid gap-3 p-4 md:grid-cols-3">
            {triggerTemplates.map((template) => (
              <Button
                key={template.name}
                variant="outline"
                className="h-auto items-start justify-start whitespace-normal p-4 text-left"
                onClick={() =>
                  setEditor({
                    initial: {
                      ...newTriggerConfig(),
                      name: template.name,
                      prompt: template.prompt,
                      source: { ...newTriggerConfig().source, expression: template.expression },
                    },
                  })
                }
              >
                <div>
                  <div className="mb-2 flex items-center justify-between gap-3 text-sm font-medium">
                    {template.name}
                    <ArrowUpRightIcon className="size-4 text-primary" />
                  </div>
                  <p className="text-xs font-normal leading-relaxed text-muted-foreground">
                    {template.description}
                  </p>
                  <p className="mt-4 text-xs font-normal text-primary">
                    {describeSchedule(template.expression)}
                  </p>
                </div>
              </Button>
            ))}
          </div>
        </div>
      ) : (
        <>
          <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div className="flex gap-1" aria-label="Filter triggers">
              {(["all", "enabled", "paused"] as const).map((option) => (
                <Button
                  key={option}
                  size="sm"
                  variant={filter === option ? "secondary" : "ghost"}
                  className="h-11 sm:h-8"
                  aria-pressed={filter === option}
                  onClick={() => setFilter(option)}
                >
                  {option.charAt(0).toUpperCase() + option.slice(1)}
                </Button>
              ))}
            </div>
            <div className="relative w-full sm:w-72">
              <MagnifyingGlassIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="h-11 pl-9 sm:h-9"
                aria-label="Search triggers"
                placeholder="Search triggers…"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          </div>
          {filtered.length === 0 ? (
            <p className="rounded-md border p-10 text-center text-sm text-muted-foreground">
              No triggers match this filter.
            </p>
          ) : (
            <>
              <div className="hidden rounded-md border bg-card md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>
                        <span className="sr-only">Enabled</span>
                      </TableHead>
                      <TableHead>Trigger</TableHead>
                      <TableHead>When</TableHead>
                      <TableHead>Run with</TableHead>
                      <TableHead>Last result</TableHead>
                      <TableHead>Next scheduled</TableHead>
                      <TableHead>
                        <span className="sr-only">Actions</span>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filtered.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className="w-14">{toggle(item)}</TableCell>
                        <TableCell className="max-w-72">
                          <Button
                            variant="link"
                            className="h-auto max-w-full justify-start p-0 text-left text-foreground"
                            onClick={() => setSelectedID(item.id)}
                          >
                            <span className="truncate">{item.name}</span>
                          </Button>
                          <p className="mt-1 max-w-64 truncate text-xs text-muted-foreground">
                            {item.prompt}
                          </p>
                        </TableCell>
                        <TableCell>
                          <p className="whitespace-nowrap text-xs">
                            {describeSchedule(item.source.expression)}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {item.source.time_zone}
                          </p>
                          <code className="mt-1 block whitespace-nowrap text-xs text-primary">
                            {item.source.expression}
                          </code>
                        </TableCell>
                        <TableCell className="max-w-44">
                          <Badge variant="outline" className="max-w-full">
                            <span className="truncate">{agentName(item.target.agent_id)}</span>
                          </Badge>
                          <p
                            className="mt-2 truncate text-xs text-muted-foreground"
                            title={workspaceName(item)}
                          >
                            {workspaceName(item)}
                          </p>
                        </TableCell>
                        <TableCell>
                          <OccurrenceStatus occurrence={item.last_occurrence} />
                          {item.last_occurrence && (
                            <p className="mt-1 text-xs text-muted-foreground">
                              {timeAgo(item.last_occurrence.created_at)}
                            </p>
                          )}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                          {item.next_fire_at
                            ? scheduledTime(item.next_fire_at, item.source.time_zone)
                            : "Paused"}
                        </TableCell>
                        <TableCell>{actions(item)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <div className="grid gap-3 md:hidden">
                {filtered.map((item) => (
                  <article key={item.id} className="rounded-md border bg-card p-4">
                    <div className="flex items-center justify-between gap-3">
                      <Button
                        variant="link"
                        className="h-auto min-w-0 justify-start p-0 text-left text-foreground"
                        onClick={() => setSelectedID(item.id)}
                      >
                        <span className="truncate">{item.name}</span>
                      </Button>
                      {toggle(item)}
                    </div>
                    <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
                      {item.prompt}
                    </p>
                    <p className="mt-4 text-xs">{describeSchedule(item.source.expression)}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.source.time_zone} · {agentName(item.target.agent_id)}
                    </p>
                    <div className="mt-3 flex items-center justify-between gap-3 border-t pt-3">
                      <OccurrenceStatus occurrence={item.last_occurrence} />
                      {actions(item)}
                    </div>
                  </article>
                ))}
              </div>
            </>
          )}
        </>
      )}

      <section
        className="mt-6 overflow-hidden rounded-md border bg-card"
        aria-labelledby="trigger-activity"
      >
        <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-4">
          <h2 id="trigger-activity" className="text-sm font-medium">
            Recent activity
          </h2>
          <span className="text-xs text-muted-foreground">
            Latest 50 occurrences · times shown locally
          </span>
        </div>
        <Activity items={history.data ?? []} loading={history.isPending} error={history.error} />
      </section>
      <p className="mt-4 text-xs leading-relaxed text-muted-foreground">
        Cron schedules run while Wingman is online. Each execution starts a fresh Session. Missed
        times and overlapping executions are skipped.
      </p>

      {editor && (
        <TriggerEditor
          trigger={editor.trigger}
          initial={editor.initial}
          onClose={() => setEditor(undefined)}
        />
      )}
      {selected && (
        <TriggerDetails
          trigger={selected}
          agentName={agentName(selected.target.agent_id)}
          onClose={() => setSelectedID(undefined)}
          onEdit={() => edit(selected)}
        />
      )}
      <Dialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleting(undefined);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {deleting?.name}?</DialogTitle>
            <DialogDescription>
              This removes the trigger and its activity history. Existing Sessions and queued or
              running work stay available.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={remove.isPending}
              onClick={() => setDeleting(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => {
                if (deleting) remove.mutate(deleting);
              }}
            >
              {remove.isPending ? "Deleting…" : "Delete trigger"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
