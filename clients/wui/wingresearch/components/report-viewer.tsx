"use client"

import { ScrollArea } from "@/components/ui/scroll-area"
import { FileText } from "lucide-react"

interface ReportViewerProps {
  content: string
  isRunning: boolean
}

export function ReportViewer({ content, isRunning }: ReportViewerProps) {
  if (!content) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-muted-foreground">
        <FileText className="h-10 w-10 mb-3 opacity-30" />
        <p className="text-sm font-medium">No report yet</p>
        <p className="text-xs mt-1">Start a research session to see the report here</p>
      </div>
    )
  }

  return (
    <ScrollArea className="h-full">
      <div className="p-5">
        <div className="prose-invert max-w-none">
          {content.split("\n").map((line, i) => {
            if (line.startsWith("# ")) {
              return (
                <h1 key={i} className="text-xl font-bold text-foreground mb-4 mt-6 first:mt-0 font-sans">
                  {line.replace("# ", "")}
                </h1>
              )
            }
            if (line.startsWith("## ")) {
              return (
                <h2 key={i} className="text-lg font-semibold text-foreground mb-3 mt-5 font-sans">
                  {line.replace("## ", "")}
                </h2>
              )
            }
            if (line.startsWith("### ")) {
              return (
                <h3 key={i} className="text-base font-medium text-foreground mb-2 mt-4 font-sans">
                  {line.replace("### ", "")}
                </h3>
              )
            }
            if (line.startsWith("- ")) {
              return (
                <li key={i} className="text-sm text-muted-foreground ml-4 mb-1 list-disc leading-relaxed">
                  {line.replace("- ", "")}
                </li>
              )
            }
            if (line.startsWith("---")) {
              return <hr key={i} className="border-border my-4" />
            }
            if (line.trim() === "") {
              return <div key={i} className="h-2" />
            }
            return (
              <p key={i} className="text-sm text-muted-foreground leading-relaxed mb-2">
                {line}
              </p>
            )
          })}
          {isRunning && (
            <span className="inline-block h-4 w-1.5 bg-primary animate-pulse rounded-sm ml-1 align-middle" />
          )}
        </div>
      </div>
    </ScrollArea>
  )
}
