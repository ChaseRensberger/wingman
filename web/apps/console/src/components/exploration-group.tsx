import { useState } from "react";
import { CircleNotchIcon } from "@phosphor-icons/react";

import { ToolActivityItem, ToolOutputParts } from "@/components/tool-activity";
import { explorationSummary, transcriptToolStatus, type TranscriptTool } from "@/lib/transcript";
import type { PermissionRequest } from "@/lib/types";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@wingman/core/components/core/collapsible";

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
    <div className="px-4 py-1" data-component="exploration-group">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger
          className="min-h-7 justify-start gap-2 py-0.5 text-xs font-normal text-muted-foreground"
          onKeyDown={(event) => {
            if (event.key === " ") event.stopPropagation();
          }}
        >
          {active ? (
            <CircleNotchIcon className="size-3.5 shrink-0 animate-spin" />
          ) : (
            <span aria-hidden="true" className="w-3.5 shrink-0 text-center">
              →
            </span>
          )}
          <span>{explorationSummary(tools)}</span>
        </CollapsibleTrigger>
        <CollapsibleContent className="pb-0">
          {tools.map(({ key, ...tool }) => (
            <ToolActivityItem key={key} {...tool} compact />
          ))}
        </CollapsibleContent>
      </Collapsible>
      {!open &&
        alwaysVisible.map(({ key, ...tool }) => <ToolActivityItem key={key} {...tool} compact />)}
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
