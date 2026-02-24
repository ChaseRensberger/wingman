"use client"

import { ScrollArea } from "@/components/ui/scroll-area"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

interface ReportViewerProps {
  content: string
  isRunning: boolean
}

export function ReportViewer({ content, isRunning }: ReportViewerProps) {
  if (!content) {
    return <div className="h-full" />
  }

  return (
    <ScrollArea className="h-full">
      <div className="p-5">
        <div className="prose prose-slate dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-li:text-foreground/90 prose-strong:text-foreground prose-a:text-primary max-w-none">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          {isRunning && (
            <span className="inline-block h-4 w-1.5 bg-primary animate-pulse rounded-sm ml-1 align-middle" />
          )}
        </div>
      </div>
    </ScrollArea>
  )
}
