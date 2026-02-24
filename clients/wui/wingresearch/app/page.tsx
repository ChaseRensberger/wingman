"use client"

import { useState, useCallback, useRef } from "react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { ReportViewer } from "@/components/report-viewer"
import { ActivityLog, type LogEntry } from "@/components/activity-log"
import { Play, Square, Radar, Map, Search, SpellCheck } from "lucide-react"

interface AgentDef {
  name: string
  description: string
  icon: typeof Map
  status: "idle" | "active" | "done"
}

const defaultAgents: AgentDef[] = [
  {
    name: "Planner",
    description: "Orchestrates research, builds outline, and delegates sections",
    icon: Map,
    status: "idle",
  },
  {
    name: "IterativeResearcher",
    description: "Fills out assigned report sections with deep research",
    icon: Search,
    status: "idle",
  },
  {
    name: "Proofreader",
    description: "Final review, proofreading, and structural improvements",
    icon: SpellCheck,
    status: "idle",
  },
]

// Simulation data for the demo
const simulationSteps = [
  { agent: "Planner", message: "Starting deep research on the topic...", type: "info" as const, delay: 800 },
  { agent: "Planner", message: "Calling perplexity_search for initial research", type: "tool" as const, delay: 1200 },
  { agent: "Planner", message: "Analyzing SERP results for agentic AI + government intelligence", type: "info" as const, delay: 2000 },
  { agent: "Planner", message: "Calling webfetch on top 3 results for deeper context", type: "tool" as const, delay: 1500 },
  { agent: "Planner", message: "Constructing report outline with 5 sections + conclusion", type: "info" as const, delay: 2000 },
  { agent: "Planner", message: "Writing initial report.md with TOC and section headers", type: "tool" as const, delay: 1000 },
  { agent: "Planner", message: "Handing off Section 1 to IterativeResearcher", type: "handoff" as const, delay: 800 },
  { agent: "IterativeResearcher", message: "Received Section 1: Introduction to Agentic AI", type: "info" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Calling perplexity_search for agentic AI definitions and frameworks", type: "tool" as const, delay: 1500 },
  { agent: "IterativeResearcher", message: "Calling webfetch to retrieve detailed technical sources", type: "tool" as const, delay: 2000 },
  { agent: "IterativeResearcher", message: "Writing Section 1 content to report.md", type: "tool" as const, delay: 1200 },
  { agent: "IterativeResearcher", message: "Section 1 complete", type: "complete" as const, delay: 500 },
  { agent: "Planner", message: "Handing off Section 2 to IterativeResearcher", type: "handoff" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Received Section 2: Government Intelligence Landscape", type: "info" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Calling perplexity_search for IC community + AI adoption", type: "tool" as const, delay: 1800 },
  { agent: "IterativeResearcher", message: "Calling webfetch on ODNI and NSA AI strategy papers", type: "tool" as const, delay: 2200 },
  { agent: "IterativeResearcher", message: "Writing Section 2 content to report.md", type: "tool" as const, delay: 1000 },
  { agent: "IterativeResearcher", message: "Section 2 complete", type: "complete" as const, delay: 500 },
  { agent: "Planner", message: "Handing off Section 3 to IterativeResearcher", type: "handoff" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Received Section 3: Open Source Enterprise (OSE)", type: "info" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Calling perplexity_search for OSE initiatives and frameworks", type: "tool" as const, delay: 1500 },
  { agent: "IterativeResearcher", message: "Calling webfetch for CIA OSE program details", type: "tool" as const, delay: 1800 },
  { agent: "IterativeResearcher", message: "Writing Section 3 content to report.md", type: "tool" as const, delay: 1200 },
  { agent: "IterativeResearcher", message: "Section 3 complete", type: "complete" as const, delay: 500 },
  { agent: "Planner", message: "Handing off Section 4 to IterativeResearcher", type: "handoff" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Received Section 4: Applications and Use Cases", type: "info" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Calling perplexity_search for agentic AI use cases in OSINT", type: "tool" as const, delay: 1800 },
  { agent: "IterativeResearcher", message: "Writing Section 4 content to report.md", type: "tool" as const, delay: 1200 },
  { agent: "IterativeResearcher", message: "Section 4 complete", type: "complete" as const, delay: 500 },
  { agent: "Planner", message: "Handing off Section 5 to IterativeResearcher", type: "handoff" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Received Section 5: Conclusion", type: "info" as const, delay: 600 },
  { agent: "IterativeResearcher", message: "Writing conclusion and final synthesis", type: "tool" as const, delay: 1500 },
  { agent: "IterativeResearcher", message: "Section 5 complete", type: "complete" as const, delay: 500 },
  { agent: "Planner", message: "All sections complete. Handing off to Proofreader.", type: "handoff" as const, delay: 800 },
  { agent: "Proofreader", message: "Received report for final review", type: "info" as const, delay: 600 },
  { agent: "Proofreader", message: "Correcting formatting, fixing typos, improving transitions", type: "tool" as const, delay: 2000 },
  { agent: "Proofreader", message: "Report finalized and polished", type: "complete" as const, delay: 800 },
  { agent: "System", message: "Research complete.", type: "complete" as const, delay: 500 },
]

const reportStages = [
  { atStep: 5, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---` },
  { atStep: 10, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.` },
  { atStep: 17, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.

## 2. Government Intelligence Landscape

The U.S. Intelligence Community (IC), comprising 18 distinct agencies and organizations, has been undergoing a significant transformation in its approach to artificial intelligence. The Office of the Director of National Intelligence (ODNI) published its AI Strategy in 2024, emphasizing the need for AI systems that can augment human analysts rather than replace them.

The National Security Agency (NSA) established its AI Security Center to lead efforts in securing AI adoption across the defense and intelligence sectors. Meanwhile, the CIA's Directorate of Digital Innovation has been investing heavily in AI capabilities that can process vast amounts of open-source and classified data simultaneously.

A critical challenge in this space is the tension between the need for transparency and explainability in AI decision-making and the inherently secretive nature of intelligence operations. Agentic AI systems add complexity to this challenge, as their autonomous decision chains can be difficult to audit and verify.` },
  { atStep: 23, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.

## 2. Government Intelligence Landscape

The U.S. Intelligence Community (IC), comprising 18 distinct agencies and organizations, has been undergoing a significant transformation in its approach to artificial intelligence. The Office of the Director of National Intelligence (ODNI) published its AI Strategy in 2024, emphasizing the need for AI systems that can augment human analysts rather than replace them.

The National Security Agency (NSA) established its AI Security Center to lead efforts in securing AI adoption across the defense and intelligence sectors. Meanwhile, the CIA's Directorate of Digital Innovation has been investing heavily in AI capabilities that can process vast amounts of open-source and classified data simultaneously.

A critical challenge in this space is the tension between the need for transparency and explainability in AI decision-making and the inherently secretive nature of intelligence operations. Agentic AI systems add complexity to this challenge, as their autonomous decision chains can be difficult to audit and verify.

## 3. Open Source Enterprise (OSE)

The Open Source Enterprise (OSE), managed by the CIA's Directorate of Digital Innovation, serves as the IC's primary hub for open-source intelligence (OSINT) collection and analysis. OSE aggregates publicly available information from news outlets, social media, academic publications, government reports, and commercial data providers.

OSE processes millions of documents daily and makes them available to analysts across all 18 IC agencies. The integration of agentic AI into OSE operations represents a paradigm shift: instead of analysts manually querying databases and reading through results, autonomous agents can continuously monitor, filter, correlate, and synthesize information across multiple sources.

The architecture naturally lends itself to agentic AI: a planner agent can identify intelligence requirements, task specialized researcher agents to investigate specific topics across open sources, and synthesize findings into actionable intelligence products. This mirrors the existing organizational workflow but at machine speed and scale.` },
  { atStep: 28, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.

## 2. Government Intelligence Landscape

The U.S. Intelligence Community (IC), comprising 18 distinct agencies and organizations, has been undergoing a significant transformation in its approach to artificial intelligence. The Office of the Director of National Intelligence (ODNI) published its AI Strategy in 2024, emphasizing the need for AI systems that can augment human analysts rather than replace them.

The National Security Agency (NSA) established its AI Security Center to lead efforts in securing AI adoption across the defense and intelligence sectors. Meanwhile, the CIA's Directorate of Digital Innovation has been investing heavily in AI capabilities that can process vast amounts of open-source and classified data simultaneously.

A critical challenge in this space is the tension between the need for transparency and explainability in AI decision-making and the inherently secretive nature of intelligence operations. Agentic AI systems add complexity to this challenge, as their autonomous decision chains can be difficult to audit and verify.

## 3. Open Source Enterprise (OSE)

The Open Source Enterprise (OSE), managed by the CIA's Directorate of Digital Innovation, serves as the IC's primary hub for open-source intelligence (OSINT) collection and analysis. OSE aggregates publicly available information from news outlets, social media, academic publications, government reports, and commercial data providers.

OSE processes millions of documents daily and makes them available to analysts across all 18 IC agencies. The integration of agentic AI into OSE operations represents a paradigm shift: instead of analysts manually querying databases and reading through results, autonomous agents can continuously monitor, filter, correlate, and synthesize information across multiple sources.

The architecture naturally lends itself to agentic AI: a planner agent can identify intelligence requirements, task specialized researcher agents to investigate specific topics across open sources, and synthesize findings into actionable intelligence products. This mirrors the existing organizational workflow but at machine speed and scale.

## 4. Applications and Use Cases

Several concrete applications of agentic AI are emerging at the intersection of government intelligence and open source analysis:

- Automated Threat Monitoring: Multi-agent systems that continuously scan global news, social media, and dark web sources for emerging threats, with specialized agents handling different languages, regions, and threat categories.

- Supply Chain Intelligence: Agentic workflows that map global supply chains by correlating corporate filings, shipping records, sanctions lists, and trade data to identify vulnerabilities and foreign dependencies.

- Influence Operation Detection: Coordinated agent teams that identify and track information operations by analyzing social media patterns, content provenance, and network topology across platforms.

- Technical Collection Augmentation: AI agents that process and correlate signals intelligence with open-source technical data, such as satellite imagery analysis combined with social media geolocation data.

- Counterproliferation Analysis: Multi-source research agents that track procurement networks for weapons of mass destruction components by monitoring trade records, academic publications, and patent filings.` },
  { atStep: 32, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.

## 2. Government Intelligence Landscape

The U.S. Intelligence Community (IC), comprising 18 distinct agencies and organizations, has been undergoing a significant transformation in its approach to artificial intelligence. The Office of the Director of National Intelligence (ODNI) published its AI Strategy in 2024, emphasizing the need for AI systems that can augment human analysts rather than replace them.

The National Security Agency (NSA) established its AI Security Center to lead efforts in securing AI adoption across the defense and intelligence sectors. Meanwhile, the CIA's Directorate of Digital Innovation has been investing heavily in AI capabilities that can process vast amounts of open-source and classified data simultaneously.

A critical challenge in this space is the tension between the need for transparency and explainability in AI decision-making and the inherently secretive nature of intelligence operations. Agentic AI systems add complexity to this challenge, as their autonomous decision chains can be difficult to audit and verify.

## 3. Open Source Enterprise (OSE)

The Open Source Enterprise (OSE), managed by the CIA's Directorate of Digital Innovation, serves as the IC's primary hub for open-source intelligence (OSINT) collection and analysis. OSE aggregates publicly available information from news outlets, social media, academic publications, government reports, and commercial data providers.

OSE processes millions of documents daily and makes them available to analysts across all 18 IC agencies. The integration of agentic AI into OSE operations represents a paradigm shift: instead of analysts manually querying databases and reading through results, autonomous agents can continuously monitor, filter, correlate, and synthesize information across multiple sources.

The architecture naturally lends itself to agentic AI: a planner agent can identify intelligence requirements, task specialized researcher agents to investigate specific topics across open sources, and synthesize findings into actionable intelligence products. This mirrors the existing organizational workflow but at machine speed and scale.

## 4. Applications and Use Cases

Several concrete applications of agentic AI are emerging at the intersection of government intelligence and open source analysis:

- Automated Threat Monitoring: Multi-agent systems that continuously scan global news, social media, and dark web sources for emerging threats, with specialized agents handling different languages, regions, and threat categories.

- Supply Chain Intelligence: Agentic workflows that map global supply chains by correlating corporate filings, shipping records, sanctions lists, and trade data to identify vulnerabilities and foreign dependencies.

- Influence Operation Detection: Coordinated agent teams that identify and track information operations by analyzing social media patterns, content provenance, and network topology across platforms.

- Technical Collection Augmentation: AI agents that process and correlate signals intelligence with open-source technical data, such as satellite imagery analysis combined with social media geolocation data.

- Counterproliferation Analysis: Multi-source research agents that track procurement networks for weapons of mass destruction components by monitoring trade records, academic publications, and patent filings.

## 5. Conclusion

The convergence of agentic AI and government intelligence operations, particularly through the lens of the Open Source Enterprise, represents one of the most consequential applications of autonomous AI systems. The ability to deploy coordinated teams of AI agents that can plan, research, analyze, and synthesize information across vast open-source datasets fundamentally changes the speed and scale at which intelligence can be produced.

However, this capability comes with significant challenges around oversight, accountability, and the potential for autonomous systems to introduce bias or errors into intelligence products that inform critical national security decisions. The intelligence community must develop robust frameworks for human-AI teaming that leverage the speed and scale of agentic systems while maintaining the judgment and accountability that human analysts provide.

As agentic AI matures, the organizations that succeed will be those that treat these systems not as replacements for human intelligence work, but as force multipliers that enable analysts to focus on higher-order thinking while AI handles the labor-intensive collection and initial synthesis of open-source information.` },
  { atStep: 36, content: `# Agentic AI in Government Intelligence and Open Source Enterprise

## Table of Contents
- 1. Introduction to Agentic AI
- 2. Government Intelligence Landscape
- 3. Open Source Enterprise (OSE)
- 4. Applications and Use Cases
- 5. Conclusion

---

## 1. Introduction to Agentic AI

Agentic AI refers to artificial intelligence systems capable of autonomous decision-making and action-taking without continuous human intervention. Unlike traditional AI models that respond to individual prompts, agentic systems can plan multi-step operations, delegate subtasks, use external tools, and adapt their approach based on intermediate results.

The core architectural pattern involves a supervisor or planner agent that orchestrates the work of specialized sub-agents, each equipped with domain-specific tools. This mirrors the organizational structures found within intelligence agencies, where analysts, collectors, and operators coordinate through a chain of command.

Key frameworks driving agentic AI development include LangGraph, CrewAI, AutoGen, and custom orchestration layers. These frameworks provide the scaffolding for tool use, memory management, and inter-agent communication that make autonomous research and analysis possible at scale.

## 2. The Government Intelligence Landscape

The U.S. Intelligence Community (IC), comprising 18 distinct agencies and organizations, has been undergoing a significant transformation in its approach to artificial intelligence. The Office of the Director of National Intelligence (ODNI) published its AI Strategy in 2024, emphasizing the need for AI systems that can augment human analysts rather than replace them.

The National Security Agency (NSA) established its AI Security Center to lead efforts in securing AI adoption across the defense and intelligence sectors. Meanwhile, the CIA's Directorate of Digital Innovation has been investing heavily in AI capabilities that can process vast amounts of open-source and classified data simultaneously.

A critical challenge in this space is the tension between the need for transparency and explainability in AI decision-making and the inherently secretive nature of intelligence operations. Agentic AI systems add complexity to this challenge, as their autonomous decision chains can be difficult to audit and verify.

## 3. The Open Source Enterprise (OSE)

The Open Source Enterprise (OSE), managed by the CIA's Directorate of Digital Innovation, serves as the IC's primary hub for open-source intelligence (OSINT) collection and analysis. OSE aggregates publicly available information from news outlets, social media, academic publications, government reports, and commercial data providers.

OSE processes millions of documents daily and makes them available to analysts across all 18 IC agencies. The integration of agentic AI into OSE operations represents a paradigm shift: instead of analysts manually querying databases and reading through results, autonomous agents can continuously monitor, filter, correlate, and synthesize information across multiple sources.

The architecture naturally lends itself to agentic AI: a planner agent can identify intelligence requirements, task specialized researcher agents to investigate specific topics across open sources, and synthesize findings into actionable intelligence products. This mirrors the existing organizational workflow but at machine speed and scale.

## 4. Applications and Use Cases

Several concrete applications of agentic AI are emerging at the intersection of government intelligence and open-source analysis:

- **Automated Threat Monitoring:** Multi-agent systems that continuously scan global news, social media, and dark web sources for emerging threats, with specialized agents handling different languages, regions, and threat categories.

- **Supply Chain Intelligence:** Agentic workflows that map global supply chains by correlating corporate filings, shipping records, sanctions lists, and trade data to identify vulnerabilities and foreign dependencies.

- **Influence Operation Detection:** Coordinated agent teams that identify and track information operations by analyzing social media patterns, content provenance, and network topology across platforms.

- **Technical Collection Augmentation:** AI agents that process and correlate signals intelligence with open-source technical data, such as satellite imagery analysis combined with social media geolocation data.

- **Counterproliferation Analysis:** Multi-source research agents that track procurement networks for weapons of mass destruction components by monitoring trade records, academic publications, and patent filings.

## 5. Conclusion

The convergence of agentic AI and government intelligence operations, particularly through the lens of the Open Source Enterprise, represents one of the most consequential applications of autonomous AI systems. The ability to deploy coordinated teams of AI agents that can plan, research, analyze, and synthesize information across vast open-source datasets fundamentally changes the speed and scale at which intelligence can be produced.

However, this capability comes with significant challenges around oversight, accountability, and the potential for autonomous systems to introduce bias or errors into intelligence products that inform critical national security decisions. The intelligence community must develop robust frameworks for human-AI teaming that leverage the speed and scale of agentic systems while maintaining the judgment and accountability that human analysts provide.

As agentic AI matures, the organizations that succeed will be those that treat these systems not as replacements for human intelligence work, but as force multipliers that enable analysts to focus on higher-order thinking while AI handles the labor-intensive collection and initial synthesis of open-source information.` },
]

function getActiveAgent(stepIndex: number): string | null {
  if (stepIndex < 0 || stepIndex >= simulationSteps.length) return null
  return simulationSteps[stepIndex].agent
}

export default function WingResearchPage() {
  const [topic, setTopic] = useState(
    "An in depth paper on agentic AI in the context of the government intelligence and OSE (open source enterprise)."
  )
  const [agents, setAgents] = useState<AgentDef[]>(defaultAgents)
  const [parallelResearchers, setParallelResearchers] = useState(3)
  const [isRunning, setIsRunning] = useState(false)
  const [logEntries, setLogEntries] = useState<LogEntry[]>([])
  const [reportContent, setReportContent] = useState("")
  const [currentStep, setCurrentStep] = useState(-1)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const stepRef = useRef(0)

  const updateAgentStatuses = useCallback((stepIdx: number) => {
    const activeAgentName = getActiveAgent(stepIdx)
    setAgents((prev) =>
      prev.map((a) => {
        if (stepIdx >= simulationSteps.length - 1) {
          return { ...a, status: "done" }
        }
        if (a.name === activeAgentName) return { ...a, status: "active" }
        // Check if agent was done in a previous step
        const lastEntryForAgent = [...simulationSteps].slice(0, stepIdx + 1).reverse().find(s => s.agent === a.name)
        if (lastEntryForAgent?.type === "complete" && a.name !== activeAgentName) {
          return { ...a, status: "done" }
        }
        if (a.name !== activeAgentName && a.status === "active") return { ...a, status: "idle" }
        return a
      })
    )
  }, [])

  const runStep = useCallback(() => {
    const idx = stepRef.current
    if (idx >= simulationSteps.length) {
      setIsRunning(false)
      setAgents((prev) => prev.map((a) => ({ ...a, status: "done" })))
      return
    }

    const step = simulationSteps[idx]
    const now = new Date()
    const timestamp = now.toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" })

    setLogEntries((prev) => [
      ...prev,
      {
        id: `log-${idx}`,
        agent: step.agent,
        message: step.message,
        timestamp,
        type: step.type,
      },
    ])

    setCurrentStep(idx)
    updateAgentStatuses(idx)

    // Update report content at specific steps
    const reportStage = reportStages.find((r) => r.atStep === idx)
    if (reportStage) {
      setReportContent(reportStage.content)
    }

    stepRef.current = idx + 1
    const nextStep = simulationSteps[idx + 1]
    timeoutRef.current = setTimeout(runStep, nextStep?.delay ?? 500)
  }, [updateAgentStatuses])

  function handleStart() {
    if (isRunning) return
    // Reset state
    setLogEntries([])
    setReportContent("")
    setCurrentStep(-1)
    stepRef.current = 0
    setAgents((prev) => prev.map((a) => ({ ...a, status: "idle" })))
    setIsRunning(true)
    timeoutRef.current = setTimeout(runStep, 500)
  }

  function handleStop() {
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    setIsRunning(false)
    setAgents((prev) => prev.map((a) => (a.status === "active" ? { ...a, status: "idle" } : a)))
  }



  return (
    <div className="flex h-screen flex-col bg-background">
      {/* Header */}
      <header className="flex items-center justify-between border-b border-border px-6 py-3 shrink-0">
        <div className="flex items-center gap-3">
          <Radar className="h-5 w-5 text-primary" />
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

      {/* Main Content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left Panel - Config */}
        <aside className="flex w-[380px] shrink-0 flex-col border-r border-border">
          {/* Topic Input */}
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

          {/* Parallel Researchers */}
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
                <span key={n} className={cn(
                  "text-[10px] font-mono",
                  n === parallelResearchers ? "text-primary" : "text-muted-foreground/50"
                )}>
                  {n}
                </span>
              ))}
            </div>
          </div>

          {/* Agent Pipeline */}
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
                      agent.status === "active"
                        ? "border-primary/40 bg-primary/[0.03]"
                        : "border-border bg-card"
                    )}
                  >
                    <div className={cn(
                      "flex h-8 w-8 shrink-0 items-center justify-center rounded-md",
                      agent.status === "active"
                        ? "bg-primary/20 text-primary"
                        : "bg-secondary text-secondary-foreground"
                    )}>
                      <Icon className="h-4 w-4" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground">{agent.name}</p>
                      <p className="text-[11px] text-muted-foreground truncate">{agent.description}</p>
                    </div>
                    <Badge
                      variant="outline"
                      className={cn("text-[10px] uppercase tracking-wider font-mono px-2 py-0 shrink-0", info.className)}
                    >
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

        {/* Right Panel - Report & Activity */}
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
                    <span className="ml-1.5 text-[10px] font-mono text-muted-foreground">
                      ({logEntries.length})
                    </span>
                  )}
                </TabsTrigger>
              </TabsList>
              {isRunning && (
                <div className="ml-auto flex items-center gap-2 text-xs text-primary">
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary animate-pulse" />
                  <span className="font-mono text-[11px]">
                    {currentStep >= 0 && currentStep < simulationSteps.length
                      ? simulationSteps[currentStep].agent
                      : "Initializing"}
                  </span>
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
