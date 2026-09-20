import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Trigger, TriggerConfig } from "@wingman-actor/client";
import { TimerIcon, ArrowRightIcon } from "@phosphor-icons/react";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";
import { Textarea } from "@wingman/core/components/core/textarea";
import { Switch } from "@wingman/core/components/core/switch";
import { Badge } from "@wingman/core/components/core/badge";
import { Field, FieldLabel } from "@wingman/core/components/core/field";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@wingman/core/components/core/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@wingman/core/components/core/sheet";
import { client } from "@/lib/client";
import { agentsQuery, workspacesQuery, queryKeys } from "@/lib/queries";
import { describeSchedule, newTriggerConfig, scheduledTime, schedulePresets } from "@/lib/triggers";

export function TriggerEditor({
  trigger,
  initial,
  onClose,
}: {
  trigger?: Trigger;
  initial?: TriggerConfig;
  onClose: () => void;
}) {
  const [value, setValue] = useState<TriggerConfig>(() =>
    trigger
      ? {
          name: trigger.name,
          prompt: trigger.prompt,
          enabled: trigger.enabled,
          source: { ...trigger.source },
          target: { ...trigger.target },
        }
      : (initial ?? newTriggerConfig()),
  );
  const [previewSource, setPreviewSource] = useState(value.source);
  const queryClient = useQueryClient();
  const agents = useQuery(agentsQuery);
  const workspaces = useQuery(workspacesQuery);
  const [customCron, setCustomCron] = useState(
    !schedulePresets.some((preset) => preset.expression === value.source.expression),
  );

  useEffect(() => {
    const timer = setTimeout(() => setPreviewSource(value.source), 300);
    return () => clearTimeout(timer);
  }, [value.source]);

  const preview = useQuery({
    queryKey: ["trigger-preview", previewSource],
    queryFn: () => client.triggers.preview(previewSource),
    retry: false,
    staleTime: 0,
  });
  const save = useMutation({
    mutationFn: () =>
      trigger
        ? client.triggers.update(trigger.id, { ...value, expected_version: trigger.version })
        : client.triggers.create(value),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.triggers });
      onClose();
    },
  });
  const selectedAgent = agents.data?.find((agent) => agent.id === value.target.agent_id);
  const previewCurrent =
    previewSource.expression === value.source.expression &&
    previewSource.time_zone === value.source.time_zone;

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) onClose();
      }}
    >
      <SheetContent className="w-full gap-0 overflow-y-auto p-0 sm:max-w-xl">
        <SheetHeader className="border-b p-5 pr-12">
          <SheetTitle>{trigger ? "Edit trigger" : "New trigger"}</SheetTitle>
          <SheetDescription>
            Choose when to run, who does the work, and what to ask.
          </SheetDescription>
        </SheetHeader>
        <form
          className="flex flex-1 flex-col"
          onSubmit={(event) => {
            event.preventDefault();
            save.mutate();
          }}
        >
          <div className="grid gap-6 p-5">
            <Field className="gap-2">
              <FieldLabel htmlFor="trigger-name">Name</FieldLabel>
              <Input
                id="trigger-name"
                required
                maxLength={120}
                autoFocus
                placeholder="Morning inbox triage"
                value={value.name}
                onChange={(event) => setValue({ ...value, name: event.target.value })}
              />
            </Field>

            <section className="grid gap-3" aria-labelledby="trigger-when">
              <div className="flex items-center justify-between">
                <h3 id="trigger-when" className="flex items-center gap-2 text-sm font-medium">
                  <TimerIcon className="size-4 text-primary" /> When
                </h3>
                <Badge variant="outline">Cron</Badge>
              </div>
              <Select
                value={customCron ? "custom" : value.source.expression}
                onValueChange={(expression) => {
                  setCustomCron(expression === "custom");
                  if (expression && expression !== "custom")
                    setValue({ ...value, source: { ...value.source, expression } });
                }}
              >
                <SelectTrigger aria-label="Schedule preset" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {schedulePresets.map((preset) => (
                    <SelectItem key={preset.expression} value={preset.expression}>
                      {preset.label}
                    </SelectItem>
                  ))}
                  <SelectItem value="custom">Custom cron expression</SelectItem>
                </SelectContent>
              </Select>
              {customCron && (
                <Field className="gap-2">
                  <FieldLabel htmlFor="trigger-cron">Cron expression</FieldLabel>
                  <Input
                    id="trigger-cron"
                    required
                    aria-describedby="trigger-cron-help"
                    aria-invalid={previewCurrent && preview.isError}
                    value={value.source.expression}
                    onChange={(event) =>
                      setValue({
                        ...value,
                        source: { ...value.source, expression: event.target.value },
                      })
                    }
                  />
                  <p id="trigger-cron-help" className="text-xs text-muted-foreground">
                    Minute · hour · day · month · weekday
                  </p>
                </Field>
              )}
              <Field className="gap-2">
                <FieldLabel htmlFor="trigger-zone">Time zone</FieldLabel>
                <Input
                  id="trigger-zone"
                  required
                  list="trigger-timezones"
                  placeholder="America/New_York"
                  value={value.source.time_zone}
                  onChange={(event) =>
                    setValue({
                      ...value,
                      source: { ...value.source, time_zone: event.target.value },
                    })
                  }
                />
                <datalist id="trigger-timezones">
                  {Array.from(
                    new Set([
                      newTriggerConfig().source.time_zone,
                      "UTC",
                      "America/New_York",
                      "America/Chicago",
                      "America/Denver",
                      "America/Los_Angeles",
                      "Europe/London",
                      "Europe/Berlin",
                      "Asia/Tokyo",
                      "Australia/Sydney",
                    ]),
                  ).map((zone) => (
                    <option key={zone} value={zone} />
                  ))}
                </datalist>
              </Field>
              <div className="rounded-md border bg-muted/30 p-3" aria-live="polite">
                <div className="mb-2 text-xs font-medium text-muted-foreground">
                  Next scheduled times
                </div>
                {!previewCurrent || preview.isPending ? (
                  <p className="text-xs text-muted-foreground">Checking schedule…</p>
                ) : preview.error ? (
                  <p className="text-xs text-destructive">{preview.error.message}</p>
                ) : (
                  <>
                    <p className="mb-2 text-sm">{describeSchedule(value.source.expression)}</p>
                    <ol className="grid gap-1.5 text-xs text-muted-foreground">
                      {preview.data?.times?.slice(0, 3).map((time) => (
                        <li key={time} className="flex items-center gap-2">
                          <ArrowRightIcon className="size-3 text-primary" />
                          {scheduledTime(time, previewSource.time_zone)}
                        </li>
                      ))}
                    </ol>
                  </>
                )}
              </div>
            </section>

            <section className="grid gap-3" aria-labelledby="trigger-target">
              <h3 id="trigger-target" className="text-sm font-medium">
                Run with
              </h3>
              <Field className="gap-2">
                <FieldLabel id="trigger-agent-label">Agent</FieldLabel>
                <Select
                  value={value.target.agent_id || null}
                  onValueChange={(agent_id) =>
                    setValue({ ...value, target: { ...value.target, agent_id: agent_id ?? "" } })
                  }
                >
                  <SelectTrigger aria-labelledby="trigger-agent-label" className="w-full">
                    <SelectValue
                      placeholder={agents.isPending ? "Loading agents…" : "Select an agent"}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {agents.data?.map((agent) => (
                      <SelectItem key={agent.id} value={agent.id}>
                        {agent.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedAgent?.model_ref && (
                  <p className="break-all text-xs text-muted-foreground">
                    {selectedAgent.model_ref}
                  </p>
                )}
                {agents.error && <p className="text-xs text-destructive">{agents.error.message}</p>}
                {agents.data?.length === 0 && (
                  <p className="text-xs text-muted-foreground">
                    Create an Agent before you create a trigger.
                  </p>
                )}
              </Field>
              <Field className="gap-2">
                <FieldLabel id="trigger-workspace-label">
                  Workspace <span className="font-normal text-muted-foreground">optional</span>
                </FieldLabel>
                <Select
                  value={value.target.workspace_id || "none"}
                  onValueChange={(workspace_id) =>
                    setValue({
                      ...value,
                      target: {
                        ...value.target,
                        workspace_id:
                          workspace_id === "none" ? undefined : (workspace_id ?? undefined),
                        working_directory: undefined,
                      },
                    })
                  }
                >
                  <SelectTrigger aria-labelledby="trigger-workspace-label" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No workspace</SelectItem>
                    {workspaces.data?.map((workspace) => (
                      <SelectItem key={workspace.id} value={workspace.id}>
                        {workspace.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {workspaces.error && (
                  <p className="text-xs text-destructive">{workspaces.error.message}</p>
                )}
              </Field>
              {!value.target.workspace_id && (
                <Field className="gap-2">
                  <FieldLabel htmlFor="trigger-directory">
                    Working directory{" "}
                    <span className="font-normal text-muted-foreground">optional</span>
                  </FieldLabel>
                  <Input
                    id="trigger-directory"
                    placeholder="/home/you/projects/wingman"
                    value={value.target.working_directory ?? ""}
                    onChange={(event) =>
                      setValue({
                        ...value,
                        target: {
                          ...value.target,
                          working_directory: event.target.value || undefined,
                        },
                      })
                    }
                  />
                </Field>
              )}
              <details className="text-xs">
                <summary className="w-fit cursor-pointer rounded-sm text-muted-foreground outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
                  Model override
                </summary>
                <Field className="mt-3 gap-2">
                  <FieldLabel htmlFor="trigger-model">Model reference</FieldLabel>
                  <Input
                    id="trigger-model"
                    placeholder="Use the Agent’s model"
                    value={value.target.model_ref ?? ""}
                    onChange={(event) =>
                      setValue({
                        ...value,
                        target: { ...value.target, model_ref: event.target.value || undefined },
                      })
                    }
                  />
                  <p className="text-xs text-muted-foreground">
                    Use provider/model. Leave empty to use the Agent’s current model.
                  </p>
                </Field>
              </details>
            </section>

            <Field className="gap-2">
              <FieldLabel htmlFor="trigger-prompt">What should the Agent do?</FieldLabel>
              <Textarea
                id="trigger-prompt"
                required
                maxLength={65536}
                className="min-h-36"
                placeholder="Describe the task, the information to use, and the result you want."
                value={value.prompt}
                onChange={(event) => setValue({ ...value, prompt: event.target.value })}
              />
            </Field>
            <div className="flex items-center justify-between gap-4 rounded-md border p-3">
              <div>
                <label htmlFor="trigger-enabled" className="cursor-pointer text-sm font-medium">
                  Enable trigger
                </label>
                <p className="mt-1 text-xs text-muted-foreground">
                  {value.enabled
                    ? "Start at the next scheduled time."
                    : "Save paused. You can still run it manually."}
                </p>
              </div>
              <Switch
                id="trigger-enabled"
                checked={value.enabled}
                onCheckedChange={(enabled) => setValue({ ...value, enabled })}
              />
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Each execution starts a fresh Session. Missed times and overlapping executions are
              skipped. Wingman must be running. The Agent’s usual permissions apply.
            </p>
            {save.error && (
              <p
                role="alert"
                className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
              >
                {save.error.message}
              </p>
            )}
          </div>
          <div className="sticky bottom-0 mt-auto flex justify-end gap-2 border-t bg-background p-4">
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              onClick={onClose}
              className="h-11 sm:h-9"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={
                save.isPending ||
                !value.target.agent_id ||
                !previewCurrent ||
                preview.isPending ||
                preview.isError
              }
              className="h-11 sm:h-9"
            >
              {save.isPending ? "Saving…" : trigger ? "Save changes" : "Create trigger"}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}
