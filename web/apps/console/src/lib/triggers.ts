import type { TriggerConfig, TriggerOccurrence } from "@wingman-actor/client";

export const schedulePresets = [
  { label: "Every hour", expression: "0 * * * *" },
  { label: "Every day at 8 AM", expression: "0 8 * * *" },
  { label: "Weekdays at 8 AM", expression: "0 8 * * 1-5" },
  { label: "Mondays at 9 AM", expression: "0 9 * * 1" },
] as const;

export function describeSchedule(expression: string): string {
  const preset = schedulePresets.find((item) => item.expression === expression);
  if (preset) return preset.label;
  const fields = expression.trim().split(/\s+/);
  if (fields.length !== 5) return "Custom schedule";
  const [minute, hour, day, month, weekday] = fields;
  if (expression === "* * * * *") return "Every minute";
  if (/^\*\/\d+$/.test(minute) && fields.slice(1).every((field) => field === "*")) {
    const interval = Number(minute.slice(2));
    if (interval > 0 && interval < 60 && 60 % interval === 0) return `Every ${interval} minutes`;
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day === "*" && month === "*") {
    const h = Number(hour),
      m = Number(minute);
    if (h > 23 || m > 59) return "Custom schedule";
    const at = `${h % 12 || 12}:${String(m).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`;
    if (weekday === "*") return `Every day at ${at}`;
    if (weekday === "1-5") return `Weekdays at ${at}`;
    const days = [
      "Sundays",
      "Mondays",
      "Tuesdays",
      "Wednesdays",
      "Thursdays",
      "Fridays",
      "Saturdays",
      "Sundays",
    ];
    if (/^[0-7]$/.test(weekday)) return `${days[Number(weekday)]} at ${at}`;
  }
  return "Custom schedule";
}

export function scheduledTime(value: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone,
  }).format(new Date(value));
}

export function occurrenceDuration(occurrence: TriggerOccurrence): string {
  if (!occurrence.started_at || !occurrence.completed_at) return "—";
  const seconds = Math.max(
    0,
    Math.round((Date.parse(occurrence.completed_at) - Date.parse(occurrence.started_at)) / 1_000),
  );
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

export function newTriggerConfig(): TriggerConfig {
  return {
    name: "",
    prompt: "",
    enabled: true,
    source: {
      type: "cron",
      expression: "0 8 * * 1-5",
      time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
    },
    target: { agent_id: "" },
  };
}

export const triggerTemplates = [
  {
    name: "Morning brief",
    expression: "0 8 * * 1-5",
    description: "Start the day with a focused summary.",
    prompt:
      "Review the information available through your tools. Summarize what needs my attention today, explain why, and suggest the next actions. State which sources you could not access.",
  },
  {
    name: "Repository health",
    expression: "0 2 * * *",
    description: "Check the project while you are away.",
    prompt:
      "Read the project instructions. Run the relevant project checks and summarize failures with file paths and useful error details. Do not change files, commit, or push.",
  },
  {
    name: "Weekly review",
    expression: "0 9 * * 1",
    description: "Review progress and choose what comes next.",
    prompt:
      "Review activity from the last seven days using the available project files and tools. Summarize completed work, open problems, and the three most useful next actions. Cite the sources you used.",
  },
] as const;
