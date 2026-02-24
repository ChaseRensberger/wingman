"use client"

import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

export interface LogEntry {
  id: string
  agent: string
  message: string
  timestamp: string
  type: "info" | "tool" | "handoff" | "complete"
}

interface ActivityLogProps {
  entries: LogEntry[]
}

const typeStyles = {
  info: "text-muted-foreground",
  tool: "text-primary",
  handoff: "text-amber-400",
  complete: "text-emerald-400",
}

const agentColors: Record<string, string> = {
  Planner: "text-primary",
  IterativeResearcher: "text-amber-400",
  Proofreader: "text-emerald-400",
  System: "text-muted-foreground",
}

export function ActivityLog({ entries }: ActivityLogProps) {
  if (entries.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-xs text-muted-foreground">Activity will appear here during research</p>
      </div>
    )
  }

  return (
    <ScrollArea className="h-full">
      <div className="p-3 space-y-1">
        {entries.map((entry) => (
          <div key={entry.id} className="flex items-start gap-2 py-1 group">
            <span className="text-[10px] text-muted-foreground font-mono shrink-0 mt-0.5 opacity-50 group-hover:opacity-100 transition-opacity">
              {entry.timestamp}
            </span>
            <span className={cn("text-[10px] font-mono font-bold shrink-0 mt-0.5 min-w-[110px]", agentColors[entry.agent] || "text-foreground")}>
              [{entry.agent}]
            </span>
            <span className={cn("text-xs leading-relaxed", typeStyles[entry.type])}>
              {entry.message}
            </span>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
