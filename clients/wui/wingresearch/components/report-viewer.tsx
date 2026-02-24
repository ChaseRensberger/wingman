"use client"

import { ScrollArea } from "@/components/ui/scroll-area"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

interface ReportViewerProps {
  content: string
  isRunning: boolean
}

function sanitizeReport(content: string): string {
  return content
    .replace(/<!--\s*SECTION:[\s\S]*?:START\s*-->/g, "")
    .replace(/<!--\s*SECTION:[\s\S]*?:END\s*-->/g, "")
    .replace(/<system-reminder>[\s\S]*?<\/system-reminder>/gi, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim()
}

export function ReportViewer({ content, isRunning }: ReportViewerProps) {
  const sanitized = sanitizeReport(content)

  if (!sanitized) {
    return <div className="h-full" />
  }

  return (
    <ScrollArea className="h-full">
      <div className="p-5">
        <div className="prose prose-slate dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-li:text-foreground/90 prose-strong:text-foreground prose-a:text-primary max-w-none">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{sanitized}</ReactMarkdown>
          {isRunning && (
            <span className="inline-block h-4 w-1.5 bg-primary animate-pulse rounded-sm ml-1 align-middle" />
          )}
        </div>
      </div>
    </ScrollArea>
  )
}
