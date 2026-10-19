import { useState } from "react";
import { Collapsible as CollapsiblePrimitive } from "@base-ui/react/collapsible";
import { ArrowRightIcon } from "@phosphor-icons/react";

import { ToolActivityItem, ToolOutputParts } from "@/components/tool-activity";
import { explorationSummary, transcriptToolStatus, type TranscriptTool } from "@/lib/transcript";
import type { PermissionRequest } from "@/lib/types";
import { Collapsible, CollapsibleContent } from "@wingman/core/components/core/collapsible";
import { Spinner } from "@wingman/core/components/core/spinner";

export function ExplorationGroup({
  tools,
  permissionRequests,
}: {
  tools: TranscriptTool[];
  permissionRequests: PermissionRequest[];
}) {
  const [open, setOpen] = useState(false);
  const active = tools.some((tool) => ["pending", "running"].includes(transcriptToolStatus(tool)));
  const alwaysVisible = tools.filter(
    (tool) =>
      transcriptToolStatus(tool) === "error" ||
      permissionRequests.some((request) =>
        request.tool_use_id || tool.call.tool_use_id
          ? request.tool_use_id === tool.call.tool_use_id
          : request.call_id === tool.call.call_id,
      ),
  );
  return (
    <div className="flex flex-col gap-1 px-4" data-component="exploration-group">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsiblePrimitive.Trigger
          className="group/exploration flex min-h-7 max-w-full items-center gap-2 rounded-sm py-0.5 text-left text-xs font-normal text-muted-foreground outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
          onKeyDown={(event) => {
            if (event.key === " ") event.stopPropagation();
          }}
        >
          {active ? (
            <Spinner size={16} className="size-4 shrink-0" label="Exploring" />
          ) : (
            <ArrowRightIcon aria-hidden="true" className="size-4 shrink-0" weight="bold" />
          )}
          <span className="transition-colors group-hover/exploration:text-foreground">
            {explorationSummary(tools)}
          </span>
        </CollapsiblePrimitive.Trigger>
        <CollapsibleContent className="pb-0">
          <div className="flex flex-col gap-1 pt-1">
            {tools.map(({ key, ...tool }) => (
              <ToolActivityItem key={key} {...tool} />
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
      {!open && alwaysVisible.map(({ key, ...tool }) => <ToolActivityItem key={key} {...tool} />)}
      {!open &&
        tools
          .filter((tool) => !alwaysVisible.includes(tool))
          .map((tool) => (
            <ToolOutputParts
              key={tool.key}
              parts={tool.result?.output.filter((part) => part.type === "image") ?? []}
              structured={undefined}
            />
          ))}
    </div>
  );
}
