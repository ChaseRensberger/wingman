"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { Play, Square, Map, Search, SpellCheck } from "lucide-react"
import Image from "next/image"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { ReportViewer } from "@/components/report-viewer"
import { ActivityLog, type LogEntry } from "@/components/activity-log"
import { buildDeepResearchDefinition } from "@/lib/deep-research-definition"
import { api, getBaseUrl, setBaseUrl, type FormationDefinition, type FormationRunEvent } from "@/lib/wingman"

interface AgentDef {
  name: string
  nodeId: string
  description: string
  icon: typeof Map
  status: "idle" | "active" | "done"
}

const defaultAgents: AgentDef[] = [
  {
    name: "Planner",
    nodeId: "planner",
    description: "Orchestrates research, builds outline, and delegates sections",
    icon: Map,
    status: "idle",
  },
  {
    name: "IterativeResearcher",
    nodeId: "iterative_research",
    description: "Fills assigned report sections with deep research",
    icon: Search,
    status: "idle",
  },
  {
    name: "Proofreader",
    nodeId: "proofreader",
    description: "Final review, proofreading, and structural improvements",
    icon: SpellCheck,
    status: "idle",
  },
]

const nodeToAgentName: Record<string, string> = {
  planner: "Planner",
  iterative_research: "IterativeResearcher",
  proofreader: "Proofreader",
}

function toTimestamp(ts?: string): string {
  const date = ts ? new Date(ts) : new Date()
  if (Number.isNaN(date.getTime())) {
    return new Date().toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
  }

  return date.toLocaleTimeString("en-US", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })
}

function normalizeBaseUrl(url: string): string {
  const trimmed = url.trim()
  const withProtocol = /^https?:\/\//i.test(trimmed) ? trimmed : `http://${trimmed}`
  const parsed = new URL(withProtocol)
  return parsed.toString().replace(/\/$/, "")
}

function summarizeOutput(output: Record<string, unknown> | undefined): string {
  if (!output) return "no output"
  const keys = Object.keys(output)
  if (keys.length === 0) return "empty output"
  return `keys: ${keys.join(", ")}`
}

export default function WingResearchPage() {
  const [topic, setTopic] = useState(
    "State of open-source local inference in 2026"
  )
  const [baseUrl, setBaseUrlState] = useState(getBaseUrl())
  const [agents, setAgents] = useState<AgentDef[]>(defaultAgents)
  const [parallelResearchers, setParallelResearchers] = useState(3)
  const [isRunning, setIsRunning] = useState(false)
  const [logEntries, setLogEntries] = useState<LogEntry[]>([])
  const [currentNode, setCurrentNode] = useState<string>("")
  const [nodeOutputs, setNodeOutputs] = useState<Record<string, Record<string, unknown>>>({})
  const [reportMarkdown, setReportMarkdown] = useState("")
  const abortRef = useRef<AbortController | null>(null)
  const formationIDRef = useRef("")

  const reportContent = reportMarkdown

  const appendLog = useCallback((entry: Omit<LogEntry, "id">) => {
    setLogEntries((prev) => [
      ...prev,
      {
        id: `${Date.now()}-${prev.length}`,
        ...entry,
      },
    ])
  }, [])

  const loadReportWithRetry = useCallback(async (formationID: string, attempts = 5) => {
    for (let i = 0; i < attempts; i += 1) {
      try {
        const report = await api.getFormationReport(formationID)
        setReportMarkdown(report.content)
        return report
      } catch {
        if (i === attempts - 1) {
          throw new Error("report.md not found for this formation")
        }
        await new Promise((resolve) => setTimeout(resolve, 700))
      }
    }

    throw new Error("report.md not found for this formation")
  }, [])

  const refreshReport = useCallback(async () => {
    const formationID = formationIDRef.current
    if (!formationID) return
    try {
      const report = await api.getFormationReport(formationID)
      setReportMarkdown(report.content)
    } catch {
      // no-op: report may not exist yet
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    const loadLatestReport = async () => {
      try {
        const formations = await api.listFormations()
        const existing = formations.find((formation) => formation.name === "deep-research")
        if (!existing || cancelled) return
        const report = await api.getFormationReport(existing.id)
        if (cancelled) return
        setReportMarkdown(report.content)
      } catch {
        // no-op: report may not exist yet
      }
    }

    void loadLatestReport()

    return () => {
      cancelled = true
    }
  }, [])

  const setNodeActive = useCallback((nodeId: string) => {
    setAgents((prev) =>
      prev.map((agent) => {
        if (agent.nodeId === nodeId) {
          return { ...agent, status: "active" }
        }
        if (agent.status === "active") {
          return { ...agent, status: "idle" }
        }
        return agent
      })
    )
  }, [])

  const markNodeDone = useCallback((nodeId: string) => {
    setAgents((prev) =>
      prev.map((agent) => (agent.nodeId === nodeId ? { ...agent, status: "done" } : agent))
    )
  }, [])

  const handleFormationEvent = useCallback(
    (event: FormationRunEvent) => {
      const timestamp = toTimestamp(event.ts)

      switch (event.type) {
        case "run_start": {
          appendLog({
            agent: "System",
            message: "Formation run started",
            timestamp,
            type: "info",
          })
          break
        }
        case "node_start": {
          const nodeID = event.node_id || ""
          setCurrentNode(nodeID)
          if (nodeID) {
            setNodeActive(nodeID)
          }
          appendLog({
            agent: nodeToAgentName[nodeID] || nodeID || "System",
            message: `Node started: ${nodeID}`,
            timestamp,
            type: "info",
          })
          break
        }
        case "node_output": {
          const nodeID = event.node_id || "unknown"
          setNodeOutputs((prev) => ({
            ...prev,
            [nodeID]: event.output || {},
          }))
          appendLog({
            agent: nodeToAgentName[nodeID] || nodeID,
            message: `Node emitted structured output (${summarizeOutput(event.output)})`,
            timestamp,
            type: "tool",
          })
          break
        }
        case "tool_call": {
          const nodeID = event.node_id || "unknown"
          const worker = event.worker ? ` [${event.worker}]` : ""
          const status = event.status ? ` (${event.status})` : ""
          const path = event.path ? ` path=${event.path}` : ""
          const error = event.error ? ` error=${event.error}` : ""
          appendLog({
            agent: nodeToAgentName[nodeID] || nodeID,
            message: `Tool${worker}: ${event.tool || "unknown"}${event.call_id ? ` [${event.call_id}]` : ""}${status}${path}${error}`,
            timestamp,
            type: event.error ? "error" : "tool",
          })

          if ((event.tool === "write" || event.tool === "edit") && event.status === "done" && !event.error && event.path) {
            void refreshReport()
          }
          break
        }
        case "edge_emit": {
          appendLog({
            agent: "System",
            message: `Handoff: ${event.from} -> ${event.to}`,
            timestamp,
            type: "handoff",
          })
          break
        }
        case "node_end": {
          const nodeID = event.node_id || ""
          if (nodeID) {
            markNodeDone(nodeID)
          }
          appendLog({
            agent: nodeToAgentName[nodeID] || nodeID || "System",
            message: `Node completed with status: ${event.status || "ok"}`,
            timestamp,
            type: "complete",
          })
          break
        }
        case "node_error": {
          const nodeID = event.node_id || "unknown"
          appendLog({
            agent: nodeToAgentName[nodeID] || nodeID,
            message: event.error || "Node failed",
            timestamp,
            type: "error",
          })
          setIsRunning(false)
          setCurrentNode("")
          break
        }
        case "run_end": {
          setIsRunning(false)
          setCurrentNode("")
          setAgents((prev) => prev.map((agent) => ({ ...agent, status: "done" })))
          appendLog({
            agent: "System",
            message: "Formation run completed",
            timestamp,
            type: "complete",
          })
          break
        }
      }
    },
    [appendLog, markNodeDone, refreshReport, setNodeActive]
  )

  const ensureDeepResearchFormation = useCallback(async (definition: FormationDefinition) => {
    const formations = await api.listFormations()
    const existing = formations.find((formation) => formation.name === "deep-research")

    if (existing) {
      const updated = await api.updateFormation(existing.id, definition)
      return updated.id
    }

    const created = await api.createFormation(definition)
    return created.id
  }, [])

  const handleStart = useCallback(async () => {
    if (isRunning || !topic.trim()) {
      return
    }

    let normalizedUrl: string
    try {
      normalizedUrl = normalizeBaseUrl(baseUrl)
    } catch {
      appendLog({
        agent: "System",
        message: "Invalid Wingman base URL",
        timestamp: toTimestamp(),
        type: "error",
      })
      return
    }

    setBaseUrl(normalizedUrl)
    setBaseUrlState(normalizedUrl)
    setAgents(defaultAgents)
    setNodeOutputs({})
    setReportMarkdown("")
    setLogEntries([])
    setCurrentNode("")
    setIsRunning(true)

    appendLog({
      agent: "System",
      message: `Connecting to ${normalizedUrl}`,
      timestamp: toTimestamp(),
      type: "info",
    })

    const controller = new AbortController()
    abortRef.current = controller

    try {
      const definition = buildDeepResearchDefinition(parallelResearchers)
      const formationID = await ensureDeepResearchFormation(definition)
      formationIDRef.current = formationID

      appendLog({
        agent: "System",
        message: `Running formation ${formationID}`,
        timestamp: toTimestamp(),
        type: "info",
      })

      await api.runFormationStream(
        formationID,
        { inputs: { topic: topic.trim() } },
        controller.signal,
        handleFormationEvent
      )

      try {
        const report = await loadReportWithRetry(formationID)
        setReportMarkdown(report.content)
        appendLog({
          agent: "System",
          message: `Loaded report from ${report.path}`,
          timestamp: toTimestamp(),
          type: "complete",
        })
      } catch (reportError) {
        appendLog({
          agent: "System",
          message: reportError instanceof Error ? reportError.message : "Report not found",
          timestamp: toTimestamp(),
          type: "error",
        })
      }
    } catch (error) {
      if (controller.signal.aborted) {
        appendLog({
          agent: "System",
          message: "Run stopped",
          timestamp: toTimestamp(),
          type: "handoff",
        })
      } else {
        appendLog({
          agent: "System",
          message: error instanceof Error ? error.message : "Run failed",
          timestamp: toTimestamp(),
          type: "error",
        })
      }
      setIsRunning(false)
      setCurrentNode("")
      setAgents((prev) => prev.map((agent) => (agent.status === "active" ? { ...agent, status: "idle" } : agent)))
    } finally {
      abortRef.current = null
    }
  }, [appendLog, baseUrl, ensureDeepResearchFormation, handleFormationEvent, isRunning, loadReportWithRetry, parallelResearchers, topic])

  const handleStop = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort()
    }
  }, [])

  return (
    <div className="flex h-screen flex-col bg-background">
      <header className="flex items-center justify-between border-b border-border px-6 py-3 shrink-0">
        <div className="flex items-center gap-3">
          <Image src="/WingmanBlue.png" alt="Wingman" width={32} height={32} className="h-8 w-8" />
          <h1 className="text-sm font-semibold text-foreground tracking-tight font-sans">WingResearch</h1>
        </div>
        <div className="flex items-center gap-2">
          {isRunning ? (
            <Button variant="destructive" size="sm" onClick={handleStop} className="h-7 text-xs gap-1.5">
              <Square className="h-3 w-3" />
              Stop
            </Button>
          ) : (
            <Button size="sm" onClick={handleStart} className="h-7 text-xs gap-1.5" disabled={!topic.trim()}>
              <Play className="h-3 w-3" />
              Start Research
            </Button>
          )}
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        <aside className="flex w-[380px] shrink-0 flex-col border-r border-border">
          <div className="border-b border-border p-4">
            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-widest mb-2 block">
              Wingman Base URL
            </label>
            <input
              type="text"
              value={baseUrl}
              onChange={(e) => setBaseUrlState(e.target.value)}
              disabled={isRunning}
              placeholder="http://127.0.0.1:2323"
              className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
            />
          </div>

          <div className="border-b border-border p-4">
            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-widest mb-2 block">
              Research Topic
            </label>
            <Textarea
              value={topic}
              onChange={(e) => setTopic(e.target.value)}
              placeholder="Enter your research topic..."
              disabled={isRunning}
              className="min-h-20 text-sm resize-none bg-background"
            />
          </div>

          <div className="border-b border-border p-4">
            <div className="flex items-center justify-between mb-3">
              <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-widest">
                Parallel Researchers
              </label>
              <span className="text-sm font-mono font-semibold text-primary">{parallelResearchers}</span>
            </div>
            <input
              type="range"
              min={1}
              max={6}
              step={1}
              value={parallelResearchers}
              disabled={isRunning}
              onChange={(e) => setParallelResearchers(Number(e.target.value))}
              className={cn(
                "w-full h-1.5 appearance-none rounded-full bg-secondary cursor-pointer",
                "[&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary [&::-webkit-slider-thumb]:shadow-sm [&::-webkit-slider-thumb]:transition-transform [&::-webkit-slider-thumb]:hover:scale-110",
                "[&::-moz-range-thumb]:h-4 [&::-moz-range-thumb]:w-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:bg-primary [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:shadow-sm",
                isRunning && "opacity-50 cursor-not-allowed"
              )}
            />
            <div className="flex justify-between mt-1.5 px-0.5">
              {[1, 2, 3, 4, 5, 6].map((n) => (
                <span
                  key={n}
                  className={cn("text-[10px] font-mono", n === parallelResearchers ? "text-primary" : "text-muted-foreground/50")}
                >
                  {n}
                </span>
              ))}
            </div>
          </div>

          <div className="flex-1 overflow-auto p-4">
            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-widest mb-3 block">
              Agent Pipeline
            </label>
            <div className="space-y-2">
              {agents.map((agent) => {
                const statusConfig = {
                  idle: { label: "Idle", className: "bg-muted text-muted-foreground border-border" },
                  active: { label: "Active", className: "bg-primary/15 text-primary border-primary/30" },
                  done: { label: "Done", className: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30" },
                }
                const info = statusConfig[agent.status]
                const Icon = agent.icon
                return (
                  <div
                    key={agent.name}
                    className={cn(
                      "flex items-center gap-3 rounded-lg border px-4 py-3 transition-colors",
                      agent.status === "active" ? "border-primary/40 bg-primary/[0.03]" : "border-border bg-card"
                    )}
                  >
                    <div
                      className={cn(
                        "flex h-8 w-8 shrink-0 items-center justify-center rounded-md",
                        agent.status === "active" ? "bg-primary/20 text-primary" : "bg-secondary text-secondary-foreground"
                      )}
                    >
                      <Icon className="h-4 w-4" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground">{agent.name}</p>
                      <p className="text-[11px] text-muted-foreground truncate">{agent.description}</p>
                    </div>
                    <Badge variant="outline" className={cn("text-[10px] uppercase tracking-wider font-mono px-2 py-0 shrink-0", info.className)}>
                      {agent.status === "active" && (
                        <span className="mr-1.5 inline-block h-1.5 w-1.5 rounded-full bg-primary animate-pulse" />
                      )}
                      {info.label}
                    </Badge>
                  </div>
                )
              })}
            </div>
          </div>
        </aside>

        <main className="flex flex-1 flex-col overflow-hidden">
          <Tabs defaultValue="report" className="flex flex-1 flex-col overflow-hidden">
            <div className="flex items-center border-b border-border px-4">
              <TabsList className="bg-transparent h-10 p-0 gap-0">
                <TabsTrigger
                  value="report"
                  className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none text-xs data-[state=active]:text-foreground px-4"
                >
                  Report
                </TabsTrigger>
                <TabsTrigger
                  value="activity"
                  className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none text-xs data-[state=active]:text-foreground px-4"
                >
                  Activity
                  {logEntries.length > 0 && (
                    <span className="ml-1.5 text-[10px] font-mono text-muted-foreground">({logEntries.length})</span>
                  )}
                </TabsTrigger>
              </TabsList>
              {isRunning && (
                <div className="ml-auto flex items-center gap-2 text-xs text-primary">
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary animate-pulse" />
                  <span className="font-mono text-[11px]">{nodeToAgentName[currentNode] || "Initializing"}</span>
                </div>
              )}
            </div>
            <TabsContent value="report" className="flex-1 overflow-hidden mt-0">
              <ReportViewer content={reportContent} isRunning={isRunning} />
            </TabsContent>
            <TabsContent value="activity" className="flex-1 overflow-hidden mt-0">
              <ActivityLog entries={logEntries} />
            </TabsContent>
          </Tabs>
        </main>
      </div>
    </div>
  )
}
