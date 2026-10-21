import { sessionQueue } from "@/lib/session-queue";
import type { SessionRun } from "@/lib/types";

export function SessionQueue({ runs }: { runs: SessionRun[] }) {
  const queued = sessionQueue(runs);
  if (!queued.length) return null;
  return (
    <div className="min-w-0 shrink-0 px-3 sm:px-4">
      <ol
        aria-label="Queued messages"
        className="mx-auto flex max-h-48 w-full max-w-4xl flex-col gap-2 overflow-y-auto py-2 text-sm"
      >
        {queued.map(({ run, changes }) => (
          <li
            key={run.id}
            className="flex min-w-0 flex-col gap-1 rounded-md border bg-muted/30 px-3 py-2 break-words"
          >
            <div className="text-xs text-muted-foreground">
              Queued · {run.agent.name || run.agent.id}
              {run.agent.model_ref ? ` · ${run.agent.model_ref}` : ""}
            </div>
            {changes.map((change) => (
              <div key={change} className="text-xs text-muted-foreground">
                {change}
              </div>
            ))}
            <div className="line-clamp-3 whitespace-pre-wrap break-words">
              {run.kind === "action" ? `/${run.action}` : run.message}
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
